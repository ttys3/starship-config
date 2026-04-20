package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDecideOutput(t *testing.T) {
	cases := []struct {
		name string
		c    *Cache
		want string
	}{
		{"nil cache", nil, iconAnon},
		{"gh private", &Cache{Source: sourceGH, Private: true}, iconPrivate},
		{"gh public", &Cache{Source: sourceGH, Private: false}, iconPublic},
		{"api private", &Cache{Source: sourceAPI, Private: true}, iconPrivate},
		{"api public", &Cache{Source: sourceAPI, Private: false}, iconPublic},
		{"anon true ignored", &Cache{Source: sourceAnon, Private: true}, iconAnon},
		{"anon false", &Cache{Source: sourceAnon, Private: false}, iconAnon},
		{"unknown source", &Cache{Source: "mystery", Private: true}, iconAnon},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideOutput(tc.c); got != tc.want {
				t.Errorf("got %q; want %q", got, tc.want)
			}
		})
	}
}

func TestShouldRefresh(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	cfg := ttlConfig{
		Private:       5 * time.Minute,
		Public:        24 * time.Hour,
		Anon:          1 * time.Hour,
		ErrorCooldown: 15 * time.Minute,
	}

	cases := []struct {
		name string
		c    *Cache
		want bool
	}{
		{"nil always refreshes", nil, true},

		{"gh private fresh (1min)", &Cache{Source: sourceGH, Private: true, UpdatedAt: now.Add(-1 * time.Minute)}, false},
		{"gh private stale (10min)", &Cache{Source: sourceGH, Private: true, UpdatedAt: now.Add(-10 * time.Minute)}, true},

		{"gh public fresh (1h)", &Cache{Source: sourceGH, Private: false, UpdatedAt: now.Add(-1 * time.Hour)}, false},
		{"gh public stale (25h)", &Cache{Source: sourceGH, Private: false, UpdatedAt: now.Add(-25 * time.Hour)}, true},

		{"anon fresh (30min)", &Cache{Source: sourceAnon, UpdatedAt: now.Add(-30 * time.Minute)}, false},
		{"anon stale (2h)", &Cache{Source: sourceAnon, UpdatedAt: now.Add(-2 * time.Hour)}, true},

		// Error cooldown wins over TTL.
		{"error cooldown suppresses even stale", &Cache{
			Source:      sourceGH,
			Private:     true,
			UpdatedAt:   now.Add(-1 * time.Hour), // stale per 5-min TTL
			LastErrorAt: now.Add(-5 * time.Minute),
		}, false},
		{"expired cooldown allows refresh", &Cache{
			Source:      sourceGH,
			Private:     true,
			UpdatedAt:   now.Add(-1 * time.Hour),
			LastErrorAt: now.Add(-20 * time.Minute),
		}, true},

		// Boundary: LastErrorAt exactly at cooldown edge — `now.Sub` >= is
		// the semantic; at the edge we permit refresh.
		{"cooldown exactly at edge", &Cache{
			Source:      sourceGH,
			Private:     true,
			UpdatedAt:   now.Add(-1 * time.Hour),
			LastErrorAt: now.Add(-15 * time.Minute),
		}, true},

		// Cooldown is only a suppressor — if TTL not elapsed it should not
		// flip a fresh cache to "refresh" just because LastErrorAt is set.
		{"fresh + past error still not refreshed", &Cache{
			Source:      sourceGH,
			Private:     true,
			UpdatedAt:   now.Add(-1 * time.Minute),
			LastErrorAt: now.Add(-1 * time.Hour),
		}, false},

		// anon + cooldown
		{"anon + cooldown still suppressed", &Cache{
			Source:      sourceAnon,
			UpdatedAt:   now.Add(-2 * time.Hour), // stale per 1h Anon TTL
			LastErrorAt: now.Add(-5 * time.Minute),
		}, false},

		// Boundary: UpdatedAt exactly at TTL -> refresh (>=).
		{"private TTL exact boundary", &Cache{
			Source:    sourceGH,
			Private:   true,
			UpdatedAt: now.Add(-5 * time.Minute),
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRefresh(tc.c, now, cfg); got != tc.want {
				t.Errorf("got %v; want %v", got, tc.want)
			}
		})
	}
}

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a_b.json")

	// Read nonexistent => nil, nil.
	got, err := readCache(path)
	if err != nil || got != nil {
		t.Fatalf("readCache nonexistent: got (%v, %v); want (nil, nil)", got, err)
	}

	orig := &Cache{
		URL:       "https://github.com/a/b",
		Private:   true,
		UpdatedAt: time.Date(2026, 4, 20, 11, 30, 0, 0, time.UTC),
		Source:    sourceGH,
		ETag:      `W/"abcd"`,
	}
	if err := writeCacheAtomic(path, orig); err != nil {
		t.Fatal(err)
	}
	got, err = readCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("got nil after write")
	}
	if got.URL != orig.URL || got.Private != orig.Private || got.Source != orig.Source || got.ETag != orig.ETag {
		t.Errorf("roundtrip mismatch: got %+v; want %+v", *got, *orig)
	}
	if !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch: got %v; want %v", got.UpdatedAt, orig.UpdatedAt)
	}
}

func TestEnvDurationOverride(t *testing.T) {
	t.Setenv("GIT_REMOTE_VISIBILITY_TTL_PRIVATE", "30s")
	t.Setenv("GIT_REMOTE_VISIBILITY_TTL_PUBLIC", "1h")
	t.Setenv("GIT_REMOTE_VISIBILITY_TTL_ANON", "10m")
	t.Setenv("GIT_REMOTE_VISIBILITY_TTL_ERROR", "5m")

	cfg := loadTTLConfig()
	if cfg.Private != 30*time.Second {
		t.Errorf("Private: %v", cfg.Private)
	}
	if cfg.Public != time.Hour {
		t.Errorf("Public: %v", cfg.Public)
	}
	if cfg.Anon != 10*time.Minute {
		t.Errorf("Anon: %v", cfg.Anon)
	}
	if cfg.ErrorCooldown != 5*time.Minute {
		t.Errorf("ErrorCooldown: %v", cfg.ErrorCooldown)
	}

	// Invalid duration falls back to default.
	t.Setenv("GIT_REMOTE_VISIBILITY_TTL_PRIVATE", "not-a-duration")
	cfg2 := loadTTLConfig()
	if cfg2.Private != 5*time.Minute {
		t.Errorf("fallback Private: %v; want 5m", cfg2.Private)
	}
}
