// Package agentfs implements vfs.FS on top of a remote Aether agent
// (docs/AGENT_PROTOCOL.md, v1).
package agentfs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// Config describes an agent share.
type Config struct {
	BaseURL string // e.g. "https://nas.lan:8097" (no /agent/v1 suffix)
	Token   string
	Share   string
	Root    string // sub-path inside the share ("" = share root)
	// Fingerprint pins the agent's certificate (lowercase hex SHA-256);
	// empty means CA verification for https URLs.
	Fingerprint string
	HTTPClient  *http.Client
	UserAgent   string
}

// AgentInfo is the /agent/v1/info payload.
type AgentInfo struct {
	Version  string  `json:"version"`
	Shares   []Share `json:"shares"`
	FFprobe  bool    `json:"ffprobe"`
	Hostname string  `json:"hostname"`
	// TLS and Fingerprint describe the agent's own HTTPS (agents ≥ M5d).
	TLS         bool   `json:"tls"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Share is one exported folder of an agent.
type Share struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// ErrUnauthorized is returned when the agent rejects the token (401/403).
var ErrUnauthorized = errors.New("agentfs: agent rejected the token (401/403): check the agent token")

const (
	apiPrefix      = "/agent/v1"
	defaultUA      = "aether-media"
	metaTimeout    = 30 * time.Second
	maxLineBytes   = 1 << 20
	maxErrorBody   = 4 << 10
	maxSkipForward = 256 << 10 // forward seeks below this reuse the open body
)

// FS is an agent-backed source.
type FS struct {
	base   *url.URL
	token  string
	share  string
	root   string // cleaned, relative to the share
	client *http.Client
	ua     string
}

var _ vfs.FS = (*FS)(nil)

// Connection discipline towards an agent (docs/AGENT_PROTOCOL.md,
// « Connexions »). An agent is often a seedbox reached over the Internet
// whose host blocks addresses opening many TCP/TLS connections: every call
// to one agent goes through a single shared transport (ClientFor), HTTP/2
// when the agent offers it (every request multiplexed on one connection),
// otherwise HTTP/1.1 keep-alive with at most MaxConnsPerAgent connections.
const (
	// MaxConnsPerAgent bounds the connections (idle + active) to one agent
	// in HTTP/1.1. It must leave room for the long-lived ones: the change
	// stream (SSE) holds one for good, each playback/remux read one more.
	MaxConnsPerAgent = 8
	// ResponseHeaderTimeout exceeds the agent's own ffprobe timeout (60 s)
	// so that a slow analysis is not mistaken for a dead agent.
	ResponseHeaderTimeout = 75 * time.Second
	maxDrainBytes         = 64 << 10
)

// DefaultClient returns a new HTTP client suitable for agents: connection
// and header timeouts but no overall timeout (bodies are long media
// streams), HTTP/2 attempted even with a custom TLS configuration (pinning),
// connections bounded per host. Callers should use ClientFor, which shares
// one per agent.
func DefaultClient() *http.Client {
	return &http.Client{Transport: wrapTransport(newTransport())}
}

func newTransport() *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = ResponseHeaderTimeout
	tr.MaxIdleConns = 4 * MaxConnsPerAgent
	tr.MaxIdleConnsPerHost = MaxConnsPerAgent
	tr.MaxConnsPerHost = MaxConnsPerAgent
	tr.IdleConnTimeout = 90 * time.Second
	// Required as soon as TLSClientConfig is set (pinned agents), or
	// net/http silently stays on HTTP/1.1.
	tr.ForceAttemptHTTP2 = true
	// Detect a dead HTTP/2 connection (address blocked, NAT expired)
	// instead of queueing every request on it.
	tr.HTTP2 = &http.HTTP2Config{SendPingTimeout: 30 * time.Second, PingTimeout: 15 * time.Second}
	return tr
}

// DrainClose reads what is left of a (small) response body, up to a bound,
// then closes it: an HTTP/1.1 connection is only reused once its body was
// read to the end, an early Close drops it.
func DrainClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, rc, maxDrainBytes)
	_ = rc.Close()
}

func drainResp(resp *http.Response) {
	DrainClose(resp.Body)
	_ = resp.Body.Close() // already closed; spelled out for bodyclose
}

func parseBase(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("agentfs: invalid agent URL %q", raw)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, apiPrefix)
	u.RawQuery, u.Fragment, u.RawPath = "", "", ""
	return u, nil
}

// New validates cfg and returns the source. No network access is made.
func New(cfg Config) (vfs.FS, error) {
	return newFS(cfg)
}

func newFS(cfg Config) (*FS, error) {
	base, err := parseBase(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.Share == "" {
		return nil, errors.New("agentfs: share is required")
	}
	root, err := vfs.CleanRel(strings.Trim(cfg.Root, "/"))
	if err != nil {
		return nil, err
	}
	c, err := clientFor(base, cfg.Fingerprint, cfg.HTTPClient)
	if err != nil {
		return nil, err
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultUA
	}
	return &FS{base: base, token: cfg.Token, share: cfg.Share, root: root, client: c, ua: ua}, nil
}

// clientFor picks the HTTP client of an agent: pinned to fingerprint when
// set (https only: a pinned agent is never reached in clear).
func clientFor(base *url.URL, fingerprint string, c *http.Client) (*http.Client, error) {
	if fingerprint != "" && base.Scheme != "https" {
		return nil, errors.New("agentfs: empreinte de certificat définie mais adresse http:// (https:// attendu)")
	}
	if c != nil {
		return PinClient(c, fingerprint), nil
	}
	return ClientFor(base.String(), fingerprint), nil
}

// remote maps a source-relative path to a share-relative one.
func (f *FS) remote(name string) (rel, remote string, err error) {
	rel, err = vfs.CleanRel(name)
	if err != nil {
		return "", "", err
	}
	return rel, vfs.Join(f.root, rel), nil
}

// local maps a share-relative path from the agent back to a source-relative
// one; ok is false when it lies outside the configured Root.
func (f *FS) local(remote string) (string, bool) {
	p, err := vfs.CleanRel(strings.TrimPrefix(remote, "/"))
	if err != nil {
		return "", false
	}
	if f.root == "" {
		return p, true
	}
	if p == f.root {
		return "", true
	}
	if rest, ok := strings.CutPrefix(p, f.root+"/"); ok {
		return rest, true
	}
	return "", false
}

func (f *FS) endpoint(ep string, q url.Values) string {
	u := *f.base
	u.Path += apiPrefix + ep
	u.RawQuery = q.Encode()
	return u.String()
}

func (f *FS) query(remote string) url.Values {
	return url.Values{"share": {f.share}, "path": {remote}}
}

func (f *FS) do(ctx context.Context, method, rawURL string, hdr http.Header) (*http.Response, error) {
	return doReq(ctx, f.client, method, rawURL, f.token, f.ua, hdr)
}

func doReq(ctx context.Context, c *http.Client, method, rawURL, token, ua string, hdr http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("agentfs: %w", err)
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %w", vfs.ErrOffline, err)
	}
	return resp, nil
}

// statusErr converts a non-success response into a vfs error and closes the
// body.
func statusErr(resp *http.Response) error {
	defer drainResp(resp)
	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxErrorBody)).Decode(&body)
	detail := body.Error
	if body.Message != "" {
		detail += ": " + body.Message
	}
	if detail == "" {
		detail = resp.Status
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("agentfs: %s: %w", detail, vfs.ErrNotExist)
	case resp.StatusCode == http.StatusBadRequest:
		return fmt.Errorf("%w: agent: %s", vfs.ErrPath, detail)
	case resp.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w: agent rate limited (%s)", vfs.ErrOffline, detail)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: agent: %s", vfs.ErrOffline, detail)
	}
	return fmt.Errorf("agentfs: unexpected response %s (%s)", resp.Status, detail)
}

func withMetaTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, metaTimeout)
}

func (f *FS) getJSON(ctx context.Context, ep, remote string, v any) error {
	ctx, cancel := withMetaTimeout(ctx)
	defer cancel()
	resp, err := f.do(ctx, http.MethodGet, f.endpoint(ep, f.query(remote)), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return statusErr(resp)
	}
	defer drainResp(resp)
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("%w: agent: bad %s response: %w", vfs.ErrOffline, ep, err)
	}
	return nil
}

// walkLine is one NDJSON line of /walk: an Entry or a terminator.
type walkLine struct {
	vfs.Entry
	Done    bool   `json:"done"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Walk streams the agent's recursive listing of dir (dir itself excluded).
func (f *FS) Walk(ctx context.Context, dir string, fn func(vfs.Entry) error) error {
	rel, remote, err := f.remote(dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	resp, err := f.do(ctx, http.MethodGet, f.endpoint("/walk", f.query(remote)), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return statusErr(resp)
	}
	// Drained after the terminator so that the connection is reused (an
	// early return reads at most maxDrainBytes more).
	defer drainResp(resp)

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), maxLineBytes)
	skipped := map[string]bool{}
	underSkipped := func(p string) bool {
		for d := path.Dir(p); d != "." && d != "/" && d != ""; d = path.Dir(d) {
			if skipped[d] {
				return true
			}
		}
		return false
	}
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var l walkLine
		if err := json.Unmarshal(line, &l); err != nil {
			return fmt.Errorf("%w: agent: malformed walk line: %w", vfs.ErrOffline, err)
		}
		switch {
		case l.Done:
			return nil
		case l.Error != "":
			msg := l.Error
			if l.Message != "" {
				msg += ": " + l.Message
			}
			if l.Error == "not_found" {
				return fmt.Errorf("agentfs: walk: %s: %w", msg, vfs.ErrNotExist)
			}
			return fmt.Errorf("%w: agent walk failed: %s", vfs.ErrOffline, msg)
		}
		p, ok := f.local(l.Path)
		if !ok || p == rel || p == "" {
			continue
		}
		if rel != "" && !strings.HasPrefix(p, rel+"/") {
			continue
		}
		if len(skipped) > 0 && underSkipped(p) {
			continue
		}
		e := l.Entry
		e.Path = p
		if err := fn(e); err != nil {
			switch {
			case errors.Is(err, fs.SkipAll):
				return nil
			case errors.Is(err, fs.SkipDir):
				if e.IsDir {
					skipped[p] = true
					continue
				}
				// SkipDir on a file skips the rest of its parent directory.
				skipped[path.Dir(p)] = true
				continue
			}
			return err
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: agent walk interrupted: %w", vfs.ErrOffline, err)
	}
	return fmt.Errorf("%w: agent walk ended without terminator", vfs.ErrOffline)
}

// Stat describes name.
func (f *FS) Stat(ctx context.Context, name string) (vfs.Entry, error) {
	rel, remote, err := f.remote(name)
	if err != nil {
		return vfs.Entry{}, err
	}
	var e vfs.Entry
	if err := f.getJSON(ctx, "/stat", remote, &e); err != nil {
		return vfs.Entry{}, err
	}
	e.Path = rel
	return e, nil
}

// ReadDir lists the direct children of dir.
func (f *FS) ReadDir(ctx context.Context, dir string) ([]vfs.Entry, error) {
	_, remote, err := f.remote(dir)
	if err != nil {
		return nil, err
	}
	var es []vfs.Entry
	if err := f.getJSON(ctx, "/readdir", remote, &es); err != nil {
		return nil, err
	}
	out := es[:0]
	for _, e := range es {
		p, ok := f.local(e.Path)
		if !ok || p == "" {
			continue
		}
		e.Path = p
		out = append(out, e)
	}
	return out, nil
}

// InputURL returns "": FFmpeg and ffprobe never connect to agents directly
// (they could not check a pinned self-signed certificate, and the token
// would appear on their command line). The server hands them a signed
// loopback URL of its internal proxy instead, which reads through this FS.
func (f *FS) InputURL(string) string { return "" }

// Ping checks the agent answers /info with this token and exports the share.
func (f *FS) Ping(ctx context.Context) error {
	info, err := Info(ctx, f.base.String(), f.token, f.client) // f.client is pinned
	if err != nil {
		return err
	}
	for _, s := range info.Shares {
		if s.Name == f.share {
			return nil
		}
	}
	return fmt.Errorf("agentfs: agent does not export share %q: %w", f.share, vfs.ErrNotExist)
}

// Close is a no-op: the transport is shared by every source of the agent
// (ClientFor) and its idle connections expire on their own; closing them
// here would force the other sources to reconnect.
func (f *FS) Close() error { return nil }

// Info fetches /agent/v1/info (used by the "test connection" endpoint). c
// must already pin the agent's certificate when it has one (ClientFor).
func Info(ctx context.Context, baseURL, token string, c *http.Client) (AgentInfo, error) {
	base, err := parseBase(baseURL)
	if err != nil {
		return AgentInfo{}, err
	}
	if c == nil {
		c = ClientFor(base.String(), "")
	}
	ctx, cancel := withMetaTimeout(ctx)
	defer cancel()
	u := *base
	u.Path += apiPrefix + "/info"
	resp, err := doReq(ctx, c, http.MethodGet, u.String(), token, defaultUA, nil)
	if err != nil {
		return AgentInfo{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return AgentInfo{}, statusErr(resp)
	}
	defer drainResp(resp)
	var info AgentInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info); err != nil {
		return AgentInfo{}, fmt.Errorf("agentfs: not an Aether agent (bad /info response): %w", err)
	}
	return info, nil
}
