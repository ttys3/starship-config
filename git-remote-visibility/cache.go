package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	iconPrivate = "🔒"
	iconPublic  = "👀"
	iconAnon    = "❓"

	sourceGH   = "gh"
	sourceAPI  = "api"
	sourceAnon = "anon"
)

// Cache is the on-disk JSON schema for a single owner/repo entry.
type Cache struct {
	URL       string    `json:"url"`
	Private   bool      `json:"private"`
	UpdatedAt time.Time `json:"updated_at"`
	Source    string    `json:"source"`         // "gh" | "api" | "anon"
	ETag      string    `json:"etag,omitempty"` // for conditional If-None-Match requests (api source)
	// LastError records the last refresh failure; used to throttle retries.
	LastError   string    `json:"last_error,omitempty"`
	LastErrorAt time.Time `json:"last_error_at,omitzero"`
}

// cacheDir returns the directory used for per-repo cache files.
// Linux: ~/.cache/git-remote-visibility/
// macOS: ~/Library/Caches/git-remote-visibility/
func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "git-remote-visibility"), nil
}

func cachePath(owner, repo string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, owner+"_"+repo+".json"), nil
}

func readCache(path string) (*Cache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// writeCacheAtomic writes the cache JSON via tmp+rename for atomicity.
// The refresh worker (Phase 3) uses this under a flock.
func writeCacheAtomic(path string, c *Cache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "cache-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// decideOutput maps a cache entry to the prompt string. Age of the cache
// does not affect the mapping — stale private/public values keep displaying
// under stale-while-revalidate; the async refresher updates them.
//
// Three states are surfaced so the user can tell a confirmed public repo
// apart from a bootstrap / failure state:
//
//	🔒 private (gh or api, Private=true)
//	👀 public  (gh or api, Private=false)
//	❓ unknown (no cache yet, or anon: no gh and no token)
func decideOutput(c *Cache) string {
	if c == nil {
		return iconAnon
	}
	switch c.Source {
	case sourceGH, sourceAPI:
		if c.Private {
			return iconPrivate
		}
		return iconPublic
	case sourceAnon:
		return iconAnon
	default:
		return iconAnon
	}
}

// ttlConfig holds the per-state TTL thresholds and error cooldown,
// each overridable via environment variables (see loadTTLConfig).
type ttlConfig struct {
	Private       time.Duration
	Public        time.Duration
	Anon          time.Duration
	ErrorCooldown time.Duration
}

func loadTTLConfig() ttlConfig {
	return ttlConfig{
		Private:       envDuration("GIT_REMOTE_VISIBILITY_TTL_PRIVATE", 5*time.Minute),
		Public:        envDuration("GIT_REMOTE_VISIBILITY_TTL_PUBLIC", 24*time.Hour),
		// anon means "no credentials available"; keep TTL very short so
		// the prompt self-heals almost immediately once the user sets
		// GITHUB_TOKEN or runs `gh auth login`. Each anon-to-anon
		// refresh is just an exec.LookPath + env read + cache write
		// (<1ms), detached from the prompt path — high cadence is free.
		Anon:          envDuration("GIT_REMOTE_VISIBILITY_TTL_ANON", 10*time.Second),
		ErrorCooldown: envDuration("GIT_REMOTE_VISIBILITY_TTL_ERROR", 15*time.Minute),
	}
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// shouldRefresh decides whether to fork an async refresh for a given cache.
// nil cache (first seen) => true.
// Error cooldown suppresses retries even if the primary TTL has elapsed.
func shouldRefresh(c *Cache, now time.Time, cfg ttlConfig) bool {
	if c == nil {
		return true
	}
	if !c.LastErrorAt.IsZero() && now.Sub(c.LastErrorAt) < cfg.ErrorCooldown {
		return false
	}
	ttl := ttlFor(c, cfg)
	return now.Sub(c.UpdatedAt) >= ttl
}

func ttlFor(c *Cache, cfg ttlConfig) time.Duration {
	switch c.Source {
	case sourceGH, sourceAPI:
		if c.Private {
			return cfg.Private
		}
		return cfg.Public
	case sourceAnon:
		return cfg.Anon
	default:
		return cfg.Anon
	}
}
