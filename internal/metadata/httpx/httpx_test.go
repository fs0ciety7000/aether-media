package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

type fakeClock struct {
	mu     sync.Mutex
	t      time.Time
	sleeps []time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

func (f *fakeClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) sleep(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sleeps = append(f.sleeps, d)
	f.t = f.t.Add(d)
	return ctx.Err()
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

func newTestClient(o Options) (*Client, *fakeClock) {
	if o.UserAgent == "" {
		o.UserAgent = "Aether/test ( https://example.org )"
	}
	c := New(o)
	fc := newFakeClock()
	c.now, c.sleep = fc.now, fc.sleep
	return c, fc
}

type payload struct {
	Name string `json:"name"`
}

func TestGetJSONOK(t *testing.T) {
	var ua, accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua, accept = r.Header.Get("User-Agent"), r.Header.Get("Accept")
		w.Write([]byte(`{"name":"Amélie"}`))
	}))
	defer srv.Close()
	c, _ := newTestClient(Options{UserAgent: "Aether/1.2 ( x )"})
	var p payload
	if err := c.GetJSON(context.Background(), srv.URL, nil, &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Amélie" || ua != "Aether/1.2 ( x )" || accept != "application/json" {
		t.Fatalf("got %+v ua=%q accept=%q", p, ua, accept)
	}
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{404, metadata.ErrNotFound},
		{401, metadata.ErrUnavailable},
		{403, metadata.ErrUnavailable},
		{400, metadata.ErrUnavailable},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"status_message":"Invalid API key: You must be granted a valid key."}`, tc.status)
		}))
		c, _ := newTestClient(Options{})
		var p payload
		err := c.GetJSON(context.Background(), srv.URL+"/movie/1?api_key=SECRET123&language=fr", nil, &p)
		srv.Close()
		if !errors.Is(err, tc.want) {
			t.Fatalf("status %d: got %v, want %v", tc.status, err, tc.want)
		}
		if strings.Contains(err.Error(), "SECRET123") {
			t.Fatalf("status %d: api key leaked: %v", tc.status, err)
		}
		if !strings.Contains(err.Error(), "api_key=REDACTED") {
			t.Fatalf("status %d: expected redacted url in %v", tc.status, err)
		}
	}
}

func TestRetryAfter429(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"name":"ok"}`))
	}))
	defer srv.Close()
	c, fc := newTestClient(Options{})
	var p payload
	if err := c.GetJSON(context.Background(), srv.URL, nil, &p); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 2 || p.Name != "ok" {
		t.Fatalf("requests=%d p=%+v", n.Load(), p)
	}
	if len(fc.sleeps) != 1 || fc.sleeps[0] != 3*time.Second {
		t.Fatalf("sleeps = %v, want [3s]", fc.sleeps)
	}
}

func TestRetryAfterHTTPDate(t *testing.T) {
	c, fc := newTestClient(Options{})
	when := fc.now().Add(5 * time.Second).Format(http.TimeFormat)
	if d, ok := parseRetryAfter(when, c.now()); d != 5*time.Second || !ok {
		t.Fatalf("got %v", d)
	}
	if d, ok := parseRetryAfter("0", c.now()); d != 0 || !ok {
		t.Fatalf("got %v %v", d, ok)
	}
	if _, ok := parseRetryAfter("soon", c.now()); ok {
		t.Fatal("unparsable should be ignored")
	}
}

func TestRetryExhausted5xx(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c, fc := newTestClient(Options{})
	var p payload
	err := c.GetJSON(context.Background(), srv.URL, nil, &p)
	if !errors.Is(err, metadata.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if n.Load() != maxTries {
		t.Fatalf("requests = %d, want %d", n.Load(), maxTries)
	}
	if len(fc.sleeps) != 2 || fc.sleeps[0] != time.Second || fc.sleeps[1] != 2*time.Second {
		t.Fatalf("sleeps = %v", fc.sleeps)
	}
}

func TestRetryAfterTooLongGivesUp(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c, _ := newTestClient(Options{})
	var p payload
	if err := c.GetJSON(context.Background(), srv.URL, nil, &p); !errors.Is(err, metadata.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if n.Load() != 1 {
		t.Fatalf("requests = %d", n.Load())
	}
}

type errTransport struct{}

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial tcp: connection refused")
}

func TestNetworkErrorRedacted(t *testing.T) {
	c, fc := newTestClient(Options{Transport: errTransport{}})
	var p payload
	err := c.GetJSON(context.Background(), "https://api.example.org/3/movie/1?api_key=TOPSECRET", nil, &p)
	if !errors.Is(err, metadata.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("key leaked: %v", err)
	}
	if len(fc.sleeps) != maxTries-1 {
		t.Fatalf("sleeps = %v", fc.sleeps)
	}
}

func TestDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html>`))
	}))
	defer srv.Close()
	c, _ := newTestClient(Options{CacheDir: t.TempDir()})
	var p payload
	if err := c.GetJSON(context.Background(), srv.URL, nil, &p); !errors.Is(err, metadata.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	c, _ := newTestClient(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var p payload
	if err := c.GetJSON(ctx, srv.URL, nil, &p); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestCache(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"name":"cached"}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	c, fc := newTestClient(Options{CacheDir: dir})
	ctx := context.Background()
	h := http.Header{"Authorization": {"Bearer eyJTOKENVALUE"}}

	for i := 0; i < 2; i++ {
		var p payload
		if err := c.GetJSON(ctx, srv.URL+"/x", h, &p); err != nil || p.Name != "cached" {
			t.Fatalf("call %d: %v %+v", i, err, p)
		}
	}
	if n.Load() != 1 {
		t.Fatalf("requests = %d, want 1 (cache hit)", n.Load())
	}

	// Another token is another key.
	var p payload
	if err := c.GetJSON(ctx, srv.URL+"/x", http.Header{"Authorization": {"Bearer other"}}, &p); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 2 {
		t.Fatalf("requests = %d, want 2", n.Load())
	}

	// 404s are cached for a day.
	for i := 0; i < 2; i++ {
		if err := c.GetJSON(ctx, srv.URL+"/missing", nil, &p); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	}
	if n.Load() != 3 {
		t.Fatalf("requests = %d, want 3", n.Load())
	}
	fc.advance(25 * time.Hour)
	if err := c.GetJSON(ctx, srv.URL+"/missing", nil, &p); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if n.Load() != 4 {
		t.Fatalf("requests = %d, want 4 (404 expired)", n.Load())
	}
	// 200 still fresh after 25h, expired after 7 days.
	if err := c.GetJSON(ctx, srv.URL+"/x", h, &p); err != nil || n.Load() != 4 {
		t.Fatalf("err=%v requests=%d", err, n.Load())
	}
	fc.advance(7 * 24 * time.Hour)
	if err := c.GetJSON(ctx, srv.URL+"/x", h, &p); err != nil || n.Load() != 5 {
		t.Fatalf("err=%v requests=%d", err, n.Load())
	}

	// Nothing on disk carries the token.
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.Contains(path, "TOKEN") {
			t.Errorf("token in cache path %s", path)
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "TOKEN") {
			t.Errorf("token in cache file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLimiterReserve(t *testing.T) {
	l := &limiter{rate: 2, burst: 2}
	t0 := time.Unix(1000, 0)
	if d := l.reserve(t0); d != 0 {
		t.Fatalf("1st: %v", d)
	}
	if d := l.reserve(t0); d != 0 {
		t.Fatalf("2nd: %v", d)
	}
	if d := l.reserve(t0); d != 500*time.Millisecond {
		t.Fatalf("3rd: %v", d)
	}
	if d := l.reserve(t0); d != time.Second {
		t.Fatalf("4th (queued): %v", d)
	}
	// After 2s the debt is paid and one token is back.
	if d := l.reserve(t0.Add(2 * time.Second)); d != 0 {
		t.Fatalf("after refill: %v", d)
	}
}

func TestClientRateLimited(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, fc := newTestClient(Options{RatePerSec: 1})
	var p payload
	for i := 0; i < 3; i++ {
		if err := c.GetJSON(context.Background(), srv.URL, nil, &p); err != nil {
			t.Fatal(err)
		}
	}
	// Burst 1: first is free, the next two each wait 1s on the fake clock.
	if len(fc.sleeps) != 2 || fc.sleeps[0] != time.Second || fc.sleeps[1] != time.Second {
		t.Fatalf("sleeps = %v", fc.sleeps)
	}
}

func TestClientRateLimitedRealClock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := New(Options{UserAgent: "t", RatePerSec: 20}) // burst 20, then 50ms apart
	start := time.Now()
	var p payload
	for i := 0; i < 23; i++ {
		if err := c.GetJSON(context.Background(), srv.URL, nil, &p); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 120*time.Millisecond {
		t.Fatalf("23 requests at 20/s took %v, limiter not applied", el)
	}
}

func TestRedact(t *testing.T) {
	cases := map[string]string{
		"https://x/3/movie/1?api_key=abc&language=fr": "https://x/3/movie/1?api_key=REDACTED&language=fr",
		"https://x/?language=fr&API_KEY=abc":          "https://x/?language=fr&API_KEY=REDACTED",
		`Get "https://x/?token=zzz": EOF`:             `Get "https://x/?token=REDACTED": EOF`,
		"https://x/?monkey=1":                         "https://x/?monkey=1",
	}
	for in, want := range cases {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}
