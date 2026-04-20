package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		a, b, want string
	}{
		{"", "", ""},
		{"a", "", "a"},
		{"", "b", "b"},
		{"a", "b", "a"}, // a wins when both non-empty
		{"", "  ", "  "},
		{"\t", "b", "\t"}, // only strict empty is treated as empty
	}
	for _, tc := range cases {
		got := firstNonEmpty(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("firstNonEmpty(%q, %q) = %q; want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

// withTestServer points apiBaseURL at an httptest.Server for the duration
// of a test and restores it after.
func withTestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	orig := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = orig })
	return srv
}

func TestTryHTTP_OK_Private(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/repos/ttys3/foo" {
			t.Errorf("path: got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer XYZ" {
			t.Errorf("auth header: got %q", got)
		}
		w.Header().Set("Etag", `W/"abc123"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"private":true,"name":"foo"}`))
	}))
	ctx := context.Background()
	private, etag, ok, err := tryHTTP(ctx, "ttys3", "foo", "XYZ", nil)
	if err != nil || !ok || !private || etag != `W/"abc123"` {
		t.Errorf("got (private=%v, etag=%q, ok=%v, err=%v)", private, etag, ok, err)
	}
}

func TestTryHTTP_OK_Public(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Etag", "xyz")
		_, _ = w.Write([]byte(`{"private":false}`))
	}))
	private, etag, ok, err := tryHTTP(context.Background(), "o", "r", "t", nil)
	if err != nil || !ok || private || etag != "xyz" {
		t.Errorf("got (private=%v, etag=%q, ok=%v, err=%v)", private, etag, ok, err)
	}
}

func TestTryHTTP_NotModified_WithPrev(t *testing.T) {
	var sawIfNoneMatch string
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawIfNoneMatch = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	prev := &Cache{Private: true, ETag: `"prev-etag"`}
	private, etag, ok, err := tryHTTP(context.Background(), "o", "r", "t", prev)
	if err != nil || !ok || !private || etag != `"prev-etag"` {
		t.Errorf("got (private=%v, etag=%q, ok=%v, err=%v)", private, etag, ok, err)
	}
	if sawIfNoneMatch != `"prev-etag"` {
		t.Errorf("If-None-Match header not forwarded: %q", sawIfNoneMatch)
	}
}

func TestTryHTTP_NotModified_NoPrev(t *testing.T) {
	// Server returns 304 even though we didn't send If-None-Match (protocol
	// violation). We must refuse and surface an error, but NOT crash.
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	_, _, ok, err := tryHTTP(context.Background(), "o", "r", "t", nil)
	if ok || err == nil {
		t.Errorf("want failure; got ok=%v err=%v", ok, err)
	}
}

func TestTryHTTP_RateLimited403(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	_, _, ok, err := tryHTTP(context.Background(), "o", "r", "t", nil)
	if ok {
		t.Errorf("want failure on 403")
	}
	if err == nil || err.Error() != "status 403" {
		t.Errorf("want 'status 403'; got %v", err)
	}
}

func TestTryHTTP_ServerError(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	_, _, ok, err := tryHTTP(context.Background(), "o", "r", "t", nil)
	if ok || err == nil {
		t.Errorf("want failure; got ok=%v err=%v", ok, err)
	}
}

func TestTryHTTP_MalformedJSON(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not-json`))
	}))
	_, _, ok, err := tryHTTP(context.Background(), "o", "r", "t", nil)
	if ok || err == nil {
		t.Errorf("want failure on malformed json; got ok=%v err=%v", ok, err)
	}
}

func TestTryHTTP_RedirectRefused(t *testing.T) {
	// CheckRedirect: ErrUseLastResponse should NOT follow; we should see the
	// 302 bubble up as a non-200 "status 302" error — the Authorization token
	// must never be resent to the redirect target.
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://evil.example/leak")
		w.WriteHeader(http.StatusFound)
	}))
	_, _, ok, err := tryHTTP(context.Background(), "o", "r", "super-secret-token", nil)
	if ok {
		t.Errorf("want failure on redirect")
	}
	if err == nil || err.Error() != "status 302" {
		t.Errorf("want 'status 302'; got %v", err)
	}
}

func TestTryHTTP_ContextTimeout(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, _, ok, err := tryHTTP(ctx, "o", "r", "t", nil)
	if ok || err == nil {
		t.Errorf("want failure on ctx timeout; got ok=%v err=%v", ok, err)
	}
}
