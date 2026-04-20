package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const refreshTimeout = 5 * time.Second

// apiBaseURL is the GitHub REST API root. Package-level var so tests can
// point it at an httptest.Server.
var apiBaseURL = "https://api.github.com"

// apiClient is a dedicated HTTP client for GitHub REST calls. It refuses
// to follow redirects so that a compromised DNS / proxy cannot trick us
// into sending the Authorization header to a non-github.com host.
// The context timeout in tryHTTP bounds per-call duration; Timeout here
// is a defense-in-depth against a hung TLS handshake that ignores ctx.
var apiClient = &http.Client{
	Timeout: refreshTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// runRefresh is invoked by the detached child forked from runPrompt.
// Invariant: never write to stdout/stderr (parent already exited) unless
// GIT_REMOTE_VISIBILITY_DEBUG=1 is set (then append to debug.log in the
// cache dir). Strategy:
//  1. flock the lock file next to the cache -> dedup concurrent refreshes.
//  2. If neither `gh` CLI nor GITHUB_TOKEN/GH_TOKEN is available,
//     write a sourceAnon marker and stop.
//  3. Try `gh api repos/{owner}/{repo} --jq .private`. If it succeeds
//     (exit 0, stdout "true"/"false"), record sourceGH.
//  4. Fallback to raw HTTPS GET with Authorization: Bearer $token and
//     If-None-Match: <prev.etag>. On 200 record sourceAPI; on 304 reuse
//     prev.Private but bump UpdatedAt.
//  5. All paths failed -> preserve prev (if any) and stamp LastError/
//     LastErrorAt to trigger the error cooldown in shouldRefresh. The
//     LastError string carries the concrete failure reason for triage.
func runRefresh(ownerRepo string) error {
	owner, repo, ok := strings.Cut(ownerRepo, "/")
	if !ok || owner == "" || repo == "" {
		return errors.New("refresh: invalid arg, want owner/repo")
	}

	path, err := cachePath(owner, repo)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	debug := newDebugLogger(filepath.Dir(path))
	defer debug.Close()
	debug.Logf("refresh start owner=%s repo=%s", owner, repo)

	lockFile, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		debug.Logf("flock busy, another worker refreshing; exit")
		return nil
	}

	prev, _ := readCache(path)
	now := time.Now().UTC()
	url := "https://github.com/" + owner + "/" + repo

	_, ghErr := exec.LookPath("gh")
	hasGH := ghErr == nil
	token := firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN"))

	if !hasGH && token == "" {
		debug.Logf("no gh, no token -> source=anon")
		return writeCacheAtomic(path, &Cache{
			URL:       url,
			Source:    sourceAnon,
			UpdatedAt: now,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	var attempts []string

	if hasGH {
		private, ok, ghAttemptErr := tryGH(ctx, owner, repo)
		if ok {
			debug.Logf("gh success: private=%v", private)
			return writeCacheAtomic(path, &Cache{
				URL:       url,
				Private:   private,
				Source:    sourceGH,
				UpdatedAt: now,
			})
		}
		attempts = append(attempts, "gh: "+ghAttemptErr.Error())
		debug.Logf("gh failed: %v", ghAttemptErr)
	}

	if token != "" {
		private, etag, ok, httpErr := tryHTTP(ctx, owner, repo, token, prev)
		if ok {
			debug.Logf("http success: private=%v etag=%q", private, etag)
			return writeCacheAtomic(path, &Cache{
				URL:       url,
				Private:   private,
				Source:    sourceAPI,
				UpdatedAt: now,
				ETag:      etag,
			})
		}
		attempts = append(attempts, "api: "+httpErr.Error())
		debug.Logf("http failed: %v", httpErr)
	}

	// All attempts failed: preserve prev state if any; stamp error cooldown.
	c := prev
	if c == nil {
		c = &Cache{URL: url, Source: sourceAnon, UpdatedAt: now}
	}
	c.LastError = strings.Join(attempts, "; ")
	if c.LastError == "" {
		c.LastError = "no_attempts"
	}
	c.LastErrorAt = now
	debug.Logf("all attempts failed; LastError=%q", c.LastError)
	return writeCacheAtomic(path, c)
}

// firstNonEmpty returns a if it is non-empty, otherwise b. Used to resolve
// the GitHub token from GITHUB_TOKEN with GH_TOKEN as a fallback.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// tryGH invokes `gh api repos/<owner>/<repo> --jq .private`. Returns
// (private, true, nil) on success. On failure returns (_, false, err)
// where err describes the failure mode (exit code, unexpected output,
// context cancel) for LastError diagnostics.
func tryGH(ctx context.Context, owner, repo string) (private, ok bool, err error) {
	cmd := exec.CommandContext(ctx, "gh", "api", "repos/"+owner+"/"+repo, "--jq", ".private")
	out, execErr := cmd.Output()
	if execErr != nil {
		if ee, ok := errors.AsType[*exec.ExitError](execErr); ok {
			return false, false, fmt.Errorf("exit %d", ee.ExitCode())
		}
		return false, false, execErr
	}
	s := strings.TrimSpace(string(out))
	switch s {
	case "true":
		return true, true, nil
	case "false":
		return false, true, nil
	default:
		return false, false, fmt.Errorf("unexpected output %q", s)
	}
}

// tryHTTP calls GitHub REST API with a bearer token. Supports conditional
// requests via If-None-Match to avoid consuming core rate limit on unchanged
// resources. Returns (private, etag, true, nil) on success; on failure the
// error explains what happened (status code, decode error, transport error).
func tryHTTP(ctx context.Context, owner, repo, token string, prev *Cache) (private bool, etag string, ok bool, err error) {
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet,
		apiBaseURL+"/repos/"+owner+"/"+repo, nil)
	if reqErr != nil {
		return false, "", false, reqErr
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "git-remote-visibility")
	if prev != nil && prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}

	resp, doErr := apiClient.Do(req)
	if doErr != nil {
		return false, "", false, doErr
	}
	defer resp.Body.Close()
	// Drain remaining bytes so idle connection can be reused by keep-alive
	// in hypothetical future multi-request runs; cheap and harmless.
	defer io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusNotModified:
		if prev != nil {
			return prev.Private, prev.ETag, true, nil
		}
		// 304 without a prev ETag means the server violated the protocol
		// (we only send If-None-Match when prev.ETag != ""). Treat as a
		// soft failure but don't surface a scary reason.
		return false, "", false, errors.New("304 without prev ETag")
	case http.StatusOK:
		var body struct {
			Private bool `json:"private"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&body); decodeErr != nil {
			return false, "", false, fmt.Errorf("decode: %w", decodeErr)
		}
		return body.Private, resp.Header.Get("Etag"), true, nil
	default:
		return false, "", false, fmt.Errorf("status %d", resp.StatusCode)
	}
}

// debugLogger appends one-line records to <cacheDir>/debug.log when
// GIT_REMOTE_VISIBILITY_DEBUG=1 is set. Silent no-op otherwise.
type debugLogger struct {
	f *os.File
}

func newDebugLogger(cacheDir string) *debugLogger {
	if os.Getenv("GIT_REMOTE_VISIBILITY_DEBUG") != "1" {
		return &debugLogger{}
	}
	f, err := os.OpenFile(filepath.Join(cacheDir, "debug.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return &debugLogger{}
	}
	return &debugLogger{f: f}
}

func (d *debugLogger) Logf(format string, args ...any) {
	if d == nil || d.f == nil {
		return
	}
	fmt.Fprintf(d.f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func (d *debugLogger) Close() {
	if d == nil || d.f == nil {
		return
	}
	_ = d.f.Close()
}
