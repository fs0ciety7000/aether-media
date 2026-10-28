package syncsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/metadata/httpx"
)

const (
	maxBody          = 32 << 20
	defaultRetryWait = 60 * time.Second
	maxRetryWait     = time.Hour
	simklLockWait    = 20 * time.Second
)

// client is a small JSON HTTP client: spaced requests (separate budgets for
// reads and writes), X-Ratelimit / 429 handling without ever retrying in a
// loop (the job queue retries later), and errors free of credentials.
type client struct {
	base   string
	ua     string
	hc     *http.Client
	reads  *httpx.Limiter
	writes *httpx.Limiter

	mu          sync.Mutex
	pausedUntil time.Time
	now         func() time.Time
}

func newClient(base, ua string, readsPerSec, writesPerSec float64, tr http.RoundTripper) *client {
	if tr == nil {
		tr = http.DefaultTransport
	}
	return &client{
		base: strings.TrimRight(base, "/"), ua: ua,
		hc:    &http.Client{Transport: tr, Timeout: 30 * time.Second},
		reads: httpx.NewLimiter(readsPerSec), writes: httpx.NewLimiter(writesPerSec),
		now: time.Now,
	}
}

// response is a raw answer, for callers that interpret statuses themselves.
type response struct {
	status int
	header http.Header
	body   []byte
}

// send performs one request. Only transport failures and rate limiting are
// errors here; HTTP statuses are left to the caller.
func (c *client) send(ctx context.Context, method, path string, h http.Header, in any) (response, error) {
	return c.doReq(ctx, method, path, h, in, false)
}

// doReq is send; with own429 a 429 is handed back as a plain response (OAuth
// device polling, where it means "poll slower") instead of pausing the client.
func (c *client) doReq(ctx context.Context, method, path string, h http.Header, in any, own429 bool) (response, error) {
	c.mu.Lock()
	wait := c.pausedUntil.Sub(c.now())
	c.mu.Unlock()
	if wait > 0 {
		return response{}, &RateLimitError{RetryAfter: wait}
	}
	lim := c.reads
	if method != http.MethodGet {
		lim = c.writes
	}
	if err := lim.Wait(ctx); err != nil {
		return response{}, err
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return response{}, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return response{}, err
	}
	for k, v := range h {
		req.Header[k] = v
	}
	// Trakt requires the JSON content type on every call (412 otherwise).
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return response{}, ctx.Err()
		}
		return response{}, &RemoteError{Msg: describe(method, path) + ": " + transportError(err)}
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return response{}, &RemoteError{Msg: describe(method, path) + ": lecture interrompue"}
	}
	c.observeQuota(res.Header.Get("X-Ratelimit"))
	if res.StatusCode == http.StatusTooManyRequests && !own429 {
		d, ok := httpx.ParseRetryAfter(res.Header.Get("Retry-After"), c.now())
		switch {
		case ok:
		case bytes.Contains(b, []byte(`"rate_limit"`)):
			d = time.Second // Simkl's per-second limit: clears almost at once
		default:
			d = defaultRetryWait
		}
		d = min(max(d, time.Second), maxRetryWait)
		c.pause(d)
		return response{}, &RateLimitError{RetryAfter: d}
	}
	return response{status: res.StatusCode, header: res.Header, body: b}, nil
}

// call sends a request and maps statuses: 2xx decodes into out (when non
// nil), 401 → ErrUnauthorized, 404 → ErrNotFound, others → RemoteError.
func (c *client) call(ctx context.Context, method, path string, h http.Header, in, out any) error {
	res, err := c.send(ctx, method, path, h, in)
	if err != nil {
		return err
	}
	return res.decode(method, path, out)
}

// decode maps a response as call does.
func (res response) decode(method, path string, out any) error {
	switch {
	case res.status >= 200 && res.status < 300:
		if out != nil && len(bytes.TrimSpace(res.body)) > 0 {
			if err := json.Unmarshal(res.body, out); err != nil {
				return &RemoteError{Status: res.status, Msg: describe(method, path) + ": réponse illisible"}
			}
		}
		return nil
	case res.status == http.StatusUnauthorized:
		return ErrUnauthorized
	case res.status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, describe(method, path))
	case res.status == http.StatusForbidden:
		return &RemoteError{Status: res.status, Msg: describe(method, path) + " : identifiants d'application refusés ou compte non autorisé"}
	case res.status == http.StatusPreconditionFailed:
		// Simkl: client_id wrong, suspended or temporarily throttled.
		return &RemoteError{Status: res.status, Msg: describe(method, path) + " : client_id refusé ou bloqué temporairement"}
	case res.status == 420:
		// Trakt: account limit exceeded (e.g. watchlist size for non-VIP).
		return &RemoteError{Status: res.status, Msg: describe(method, path) + " : limite du compte distant atteinte"}
	case res.status == http.StatusBadRequest && bytes.Contains(res.body, []byte(`"RATE_LIMIT"`)):
		// Simkl's per-user write lock (~20 s): retry shortly, without pausing
		// the whole client.
		return &RateLimitError{RetryAfter: simklLockWait}
	}
	return &RemoteError{Status: res.status, Msg: describe(method, path)}
}

func (c *client) pause(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.now().Add(d); t.After(c.pausedUntil) {
		c.pausedUntil = t
	}
}

// observeQuota reads Trakt's X-Ratelimit header
// ({"name":…,"period":300,"limit":1000,"remaining":0,"until":"…"}) and stops
// sending until the window resets when the quota is exhausted.
func (c *client) observeQuota(v string) {
	if v == "" {
		return
	}
	var q struct {
		Remaining *int   `json:"remaining"`
		Until     string `json:"until"`
	}
	if json.Unmarshal([]byte(v), &q) != nil || q.Remaining == nil || *q.Remaining > 0 {
		return
	}
	if t := parseTime(q.Until); !t.IsZero() {
		if d := t.Sub(c.now()); d > 0 {
			c.pause(min(d, maxRetryWait))
		}
	}
}

// describe names a request without its query string (which may carry a
// client id).
func describe(method, path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return method + " " + path
}

func transportError(err error) string {
	var ue interface{ Timeout() bool }
	if errors.As(err, &ue) && ue.Timeout() {
		return "délai dépassé"
	}
	return "connexion impossible"
}
