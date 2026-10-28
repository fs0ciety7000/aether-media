// Package httpx is the HTTP client shared by remote metadata providers:
// User-Agent, token-bucket rate limiting, on-disk response cache, retries
// with Retry-After, and mapping of HTTP failures onto metadata.ErrNotFound /
// metadata.ErrUnavailable. API keys never appear in returned errors.
package httpx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

const (
	defaultTTL         = 7 * 24 * time.Hour
	notFoundTTL        = 24 * time.Hour
	defaultTimeout     = 20 * time.Second
	maxTries           = 3
	maxRetryAfter      = 2 * time.Minute
	maxBodyBytes       = 16 << 20
	errorSnippetLength = 200
)

// Options configures a Client.
type Options struct {
	UserAgent  string            // required, e.g. "Aether/<version> ( https://github.com/fs0ciety7000/aether-media )"
	RatePerSec float64           // token bucket refill rate; burst = max(1, int(rate)); <= 0 = unlimited
	CacheDir   string            // on-disk response cache ("" = no cache)
	CacheTTL   time.Duration     // default 7 days; 404 responses are cached 1 day
	Timeout    time.Duration     // per-request timeout, default 20s
	Transport  http.RoundTripper // nil = http.DefaultTransport
}

// Client is safe for concurrent use.
type Client struct {
	ua       string
	hc       *http.Client
	cacheDir string
	ttl      time.Duration
	lim      *limiter

	// Overridable in tests.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

// New builds a Client. One Client per remote service, so that each gets its
// own rate limit.
func New(o Options) *Client {
	if o.CacheTTL <= 0 {
		o.CacheTTL = defaultTTL
	}
	if o.Timeout <= 0 {
		o.Timeout = defaultTimeout
	}
	tr := o.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	c := &Client{
		ua:       o.UserAgent,
		hc:       &http.Client{Transport: tr, Timeout: o.Timeout},
		cacheDir: o.CacheDir,
		ttl:      o.CacheTTL,
		now:      time.Now,
		sleep:    sleepCtx,
	}
	if o.RatePerSec > 0 {
		burst := max(1, int(o.RatePerSec))
		c.lim = &limiter{rate: o.RatePerSec, burst: float64(burst)}
	}
	return c
}

// GetJSON GETs url and decodes the JSON body into out.
//
// Errors: 404 → metadata.ErrNotFound; 401/403, exhausted 429/5xx retries,
// network and decoding failures → metadata.ErrUnavailable (wrapped with a
// redacted detail). A cancelled ctx returns ctx.Err().
func (c *Client) GetJSON(ctx context.Context, url string, header http.Header, out any) error {
	h := header.Clone()
	if h == nil {
		h = http.Header{}
	}
	if h.Get("Accept") == "" {
		h.Set("Accept", "application/json")
	}
	body, fromCache, err := c.get(ctx, url, h)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		if fromCache {
			c.dropCache(url, h)
		}
		return fmt.Errorf("%w: GET %s: decode: %w", metadata.ErrUnavailable, Redact(url), err)
	}
	if !fromCache {
		c.storeCache(url, h, http.StatusOK, body)
	}
	return nil
}

// Get GETs url and returns the raw body of a 200 response, with the same
// error mapping as GetJSON. The body is cached as-is.
func (c *Client) Get(ctx context.Context, url string, header http.Header) ([]byte, error) {
	h := header.Clone()
	if h == nil {
		h = http.Header{}
	}
	body, fromCache, err := c.get(ctx, url, h)
	if err == nil && !fromCache {
		c.storeCache(url, h, http.StatusOK, body)
	}
	return body, err
}

// get serves from cache or the network. Successful bodies are not stored
// here (GetJSON only caches what decodes); 404s are.
func (c *Client) get(ctx context.Context, url string, h http.Header) ([]byte, bool, error) {
	if e, ok := c.loadCache(url, h); ok {
		if e.Status == http.StatusNotFound {
			return nil, true, fmt.Errorf("%w: GET %s (cached 404)", metadata.ErrNotFound, Redact(url))
		}
		return e.Body, true, nil
	}

	var lastErr error
	for attempt := 0; attempt < maxTries; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, false, err
		}
		status, body, retryAfter, err := c.do(ctx, url, h)
		if err != nil {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			// Network errors: retry like a 5xx.
			lastErr = fmt.Errorf("%w: GET %s: %s", metadata.ErrUnavailable, Redact(url), Redact(err.Error()))
			if attempt+1 < maxTries {
				if err := c.sleep(ctx, backoff(attempt)); err != nil {
					return nil, false, err
				}
			}
			continue
		}
		switch {
		case status >= 200 && status < 300:
			return body, false, nil
		case status == http.StatusNotFound:
			c.storeCache(url, h, status, nil)
			return nil, false, fmt.Errorf("%w: GET %s", metadata.ErrNotFound, Redact(url))
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			return nil, false, fmt.Errorf("%w: GET %s: status %d (check API key): %s",
				metadata.ErrUnavailable, Redact(url), status, snippet(body))
		case status == http.StatusTooManyRequests || status >= 500:
			lastErr = fmt.Errorf("%w: GET %s: status %d: %s", metadata.ErrUnavailable, Redact(url), status, snippet(body))
			if attempt+1 >= maxTries {
				break
			}
			d := backoff(attempt)
			if retryAfter >= 0 {
				d = retryAfter
			}
			if d > maxRetryAfter {
				return nil, false, fmt.Errorf("%w (retry after %s)", lastErr, d)
			}
			if err := c.sleep(ctx, d); err != nil {
				return nil, false, err
			}
		default:
			return nil, false, fmt.Errorf("%w: GET %s: unexpected status %d: %s",
				metadata.ErrUnavailable, Redact(url), status, snippet(body))
		}
	}
	return nil, false, lastErr
}

// do performs one attempt. retryAfter is -1 when the server gave none.
func (c *Client) do(ctx context.Context, url string, h http.Header) (int, []byte, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, 0, err
	}
	for k, v := range h {
		req.Header[k] = v
	}
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, 0, err
	}
	ra, ok := parseRetryAfter(resp.Header.Get("Retry-After"), c.now())
	if !ok {
		ra = -1
	}
	return resp.StatusCode, body, ra, nil
}

func (c *Client) wait(ctx context.Context) error {
	if c.lim == nil {
		return ctx.Err()
	}
	if d := c.lim.reserve(c.now()); d > 0 {
		return c.sleep(ctx, d)
	}
	return ctx.Err()
}

func backoff(attempt int) time.Duration { return time.Second << attempt }

// parseRetryAfter reads delay-seconds or an HTTP date; ok is false when the
// header is absent or unparsable.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(0, t.Sub(now)), true
	}
	return 0, false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// limiter is a token bucket handing out reservations: a caller takes a token
// now and is told how long to wait for it, so concurrent callers queue.
type limiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func (l *limiter) reserve(now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.last.IsZero() {
		l.tokens, l.last = l.burst, now
	}
	if el := now.Sub(l.last); el > 0 {
		l.tokens = min(l.burst, l.tokens+el.Seconds()*l.rate)
		l.last = now
	}
	l.tokens--
	if l.tokens >= 0 {
		return 0
	}
	return time.Duration(-l.tokens / l.rate * float64(time.Second))
}

// ---- cache ----

// keyHeaders are the request headers that change the response. Their values
// only ever enter the cache key through sha256.
var keyHeaders = []string{"Accept", "Accept-Language", "Authorization"}

type cacheEntry struct {
	Status int       `json:"status"`
	Time   time.Time `json:"time"`
	Body   []byte    `json:"body,omitempty"`
}

func (c *Client) cachePath(url string, h http.Header) string {
	s := sha256.New()
	io.WriteString(s, http.MethodGet+"\n"+url+"\n")
	names := append([]string(nil), keyHeaders...)
	sort.Strings(names)
	for _, n := range names {
		if v := h.Values(n); len(v) > 0 {
			io.WriteString(s, n+":"+strings.Join(v, ",")+"\n")
		}
	}
	k := hex.EncodeToString(s.Sum(nil))
	return filepath.Join(c.cacheDir, k[:2], k+".json")
}

func (c *Client) loadCache(url string, h http.Header) (cacheEntry, bool) {
	var e cacheEntry
	if c.cacheDir == "" {
		return e, false
	}
	b, err := os.ReadFile(c.cachePath(url, h))
	if err != nil || json.Unmarshal(b, &e) != nil {
		return e, false
	}
	ttl := c.ttl
	if e.Status == http.StatusNotFound {
		ttl = min(ttl, notFoundTTL)
	}
	if age := c.now().Sub(e.Time); age < 0 || age > ttl {
		return e, false
	}
	return e, true
}

// storeCache is best effort: a cache that cannot be written only costs a
// request next time.
func (c *Client) storeCache(url string, h http.Header, status int, body []byte) {
	if c.cacheDir == "" {
		return
	}
	b, err := json.Marshal(cacheEntry{Status: status, Time: c.now().UTC(), Body: body})
	if err != nil {
		return
	}
	p := c.cachePath(url, h)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), p) != nil {
		_ = os.Remove(tmp.Name())
	}
}

func (c *Client) dropCache(url string, h http.Header) {
	if c.cacheDir != "" {
		_ = os.Remove(c.cachePath(url, h))
	}
}

// ---- redaction ----

var secretParamRe = regexp.MustCompile(`(?i)\b((?:api_key|apikey|api-key|access_token|token|key)=)[^&\s"']*`)

// Redact masks credential query parameters (api_key=…, token=…) in s, which
// may be a URL or an error message containing one.
func Redact(s string) string {
	return secretParamRe.ReplaceAllString(s, "${1}REDACTED")
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > errorSnippetLength {
		s = s[:errorSnippetLength] + "…"
	}
	return Redact(s)
}
