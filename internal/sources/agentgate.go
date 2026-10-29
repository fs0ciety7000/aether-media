package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
	"github.com/fs0ciety7000/aether-media/internal/vfs/agentfs"
)

// Call discipline towards a remote agent. A seedbox host may block an
// address that opens many connections (the server then times out on every
// dial), so each agent gets:
//
//   - a concurrency limit for analyses and scan walks (AgentConcurrency),
//     independent of the number of probe workers, so that a slow or
//     distant agent does not hold up the others;
//   - a circuit breaker: the first network failure (dial timeout, refused,
//     reset) marks the agent offline; calls then fail at once with an
//     AgentDownError (jobs are postponed, not retried) while a single
//     probe of /agent/v1/info is scheduled after agentBackoff delays; its
//     success brings the agent back online.

// DefaultAgentConcurrency is the default of Manager.AgentConcurrency.
const DefaultAgentConcurrency = 2

// agentBackoff are the delays between probes of an unreachable agent (the
// last one repeats). A variable for tests.
var agentBackoff = []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute}

const agentProbeTimeout = 20 * time.Second

// AgentDownError is returned without any network access while an agent is
// considered unreachable. It matches vfs.ErrOffline.
type AgentDownError struct {
	URL   string
	Until time.Time // next probe of the agent
	Err   error     // the failure that opened the breaker (or the last probe's)
}

func (e *AgentDownError) Error() string {
	return fmt.Sprintf("agent %s injoignable, prochain essai à %s : %v", e.URL, e.Until.Format("15:04:05"), e.Err)
}

func (e *AgentDownError) Unwrap() []error { return []error{vfs.ErrOffline, e.Err} }

// RetryIn is how long to wait before calling the agent again.
func (e *AgentDownError) RetryIn() time.Duration { return max(time.Until(e.Until), time.Second) }

// isNetFailure reports a failure to reach the agent at all: dial, TLS or
// read/write errors on the connection, transport timeouts. HTTP errors
// answered by the agent, a certificate mismatch, or the caller giving up
// (context) are not.
func isNetFailure(err error) bool {
	if err == nil || errors.Is(err, agentfs.ErrFingerprintMismatch) || errors.Is(err, agentfs.ErrUnauthorized) {
		return false
	}
	if _, ok := errors.AsType[*net.OpError](err); ok {
		return true // includes dial "i/o timeout"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	ne, ok := errors.AsType[net.Error](err)
	return ok && ne.Timeout() // TLS handshake timeout…
}

// agentKey identifies an agent endpoint (as agentfs.ClientFor does).
func agentKey(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

type agentGate struct {
	m   *Manager
	key string
	sem chan struct{}

	mu               sync.Mutex
	url, fp, token   string
	agentID          int64
	down             bool
	level            int
	until            time.Time
	lastErr          error
	timer            *time.Timer
	probing, stopped bool
}

// gate returns the gate of the agent at ac.URL, refreshing the credentials
// its probe uses.
func (m *Manager) gate(ac AgentConfig, sec Secrets) *agentGate {
	k := agentKey(ac.URL)
	m.gmu.Lock()
	g, ok := m.gates[k]
	if !ok {
		n := m.AgentConcurrency
		if n < 1 {
			n = DefaultAgentConcurrency
		}
		g = &agentGate{m: m, key: k, sem: make(chan struct{}, n), stopped: m.gatesStopped}
		m.gates[k] = g
	}
	m.gmu.Unlock()
	g.mu.Lock()
	g.url, g.fp, g.token = ac.URL, ac.Fingerprint, sec.Token
	if ac.AgentID != 0 {
		g.agentID = ac.AgentID
	}
	g.mu.Unlock()
	return g
}

// stopGates cancels pending probes (Manager.Close).
func (m *Manager) stopGates() {
	m.gmu.Lock()
	m.gatesStopped = true
	gs := make([]*agentGate, 0, len(m.gates))
	for _, g := range m.gates {
		gs = append(gs, g)
	}
	m.gmu.Unlock()
	for _, g := range gs {
		g.mu.Lock()
		g.stopped = true
		if g.timer != nil {
			g.timer.Stop()
		}
		g.mu.Unlock()
	}
}

// AgentDown reports whether the agent at rawURL is behind an open breaker.
func (m *Manager) AgentDown(rawURL string) bool {
	m.gmu.Lock()
	g := m.gates[agentKey(rawURL)]
	m.gmu.Unlock()
	return g != nil && g.check() != nil
}

// check fails fast while the agent is down.
func (g *agentGate) check() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.down {
		return &AgentDownError{URL: g.url, Until: g.until, Err: g.lastErr}
	}
	return nil
}

// acquire takes one of the agent's concurrency slots.
func (g *agentGate) acquire(ctx context.Context) (release func(), err error) {
	if err := g.check(); err != nil {
		return nil, err
	}
	select {
	case g.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release = func() { <-g.sem }
	if err := g.check(); err != nil { // went down while we waited
		release()
		return nil, err
	}
	return release, nil
}

// done records the outcome of a call made with ctx: a network failure opens
// the breaker and is returned as an AgentDownError.
func (g *agentGate) done(ctx context.Context, err error) error {
	if err == nil || ctx.Err() != nil || !isNetFailure(err) {
		return err
	}
	g.trip(err)
	if de := g.check(); de != nil {
		return de
	}
	return err
}

func (g *agentGate) trip(err error) {
	g.mu.Lock()
	if g.down || g.stopped {
		g.mu.Unlock()
		return
	}
	g.down, g.level, g.lastErr = true, 0, err
	g.until = time.Now().Add(agentBackoff[0])
	g.scheduleLocked()
	url, fp, id, until := g.url, g.fp, g.agentID, g.until
	g.mu.Unlock()
	// Connections to it are probably dead: do not queue requests on them.
	agentfs.ClientFor(url, fp).CloseIdleConnections()
	g.m.log.Warn("agent injoignable : appels suspendus jusqu'au prochain essai", "url", url, "essai", until.Format("15:04:05"), "err", err)
	g.m.agentTransition(g, id, false, err)
}

func (g *agentGate) scheduleLocked() {
	if g.timer != nil {
		g.timer.Stop()
	}
	g.timer = time.AfterFunc(time.Until(g.until), g.probe)
}

// probe is the single request made to a down agent, once per backoff step.
func (g *agentGate) probe() {
	g.mu.Lock()
	if !g.down || g.stopped || g.probing {
		g.mu.Unlock()
		return
	}
	g.probing = true
	url, fp, token := g.url, g.fp, g.token
	g.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), agentProbeTimeout)
	_, err := g.m.factory.AgentInfo(ctx, url, token, fp)
	failed := err != nil && (ctx.Err() != nil || isNetFailure(err) || errors.Is(err, agentfs.ErrFingerprintMismatch))
	cancel()

	g.mu.Lock()
	g.probing = false
	if !g.down || g.stopped {
		g.mu.Unlock()
		return
	}
	if failed {
		g.level = min(g.level+1, len(agentBackoff)-1)
		g.until = time.Now().Add(agentBackoff[g.level])
		g.lastErr = err
		g.scheduleLocked()
		until := g.until
		g.mu.Unlock()
		g.m.log.Info("agent toujours injoignable", "url", url, "prochain essai", until.Format("15:04:05"), "err", err)
		return
	}
	g.mu.Unlock()
	g.up()
}

// up closes the breaker (a probe or another call reached the agent).
func (g *agentGate) up() {
	g.mu.Lock()
	if !g.down {
		g.mu.Unlock()
		return
	}
	g.down, g.level, g.lastErr = false, 0, nil
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	url, id := g.url, g.agentID
	g.mu.Unlock()
	g.m.log.Info("agent de nouveau joignable : reprise des appels", "url", url)
	g.m.agentTransition(g, id, true, nil)
	if f := g.m.OnAgentOnline; f != nil {
		f()
	}
}

// agentTransition records an agent going offline / online on the paired
// agent row and on its sources, and notifies the clients.
func (m *Manager) agentTransition(g *agentGate, agentID int64, online bool, cause error) {
	ctx := context.Background()
	status, msg := "online", ""
	var srcErr error
	if !online {
		status = "offline"
		srcErr = g.check()
		if srcErr == nil {
			srcErr = cause
		}
		msg = srcErr.Error()
	}
	if agentID != 0 {
		if err := m.store.SetAgentStatus(ctx, db.SetAgentStatusParams{Status: status, LastError: msg, Column3: status, ID: agentID}); err == nil && m.hub != nil {
			m.hub.PublishAdmin("agent.status", map[string]any{"id": agentID, "status": status, "lastError": msg})
		}
	}
	srcs, err := m.store.ListAllSources(ctx)
	if err != nil {
		return
	}
	for _, src := range srcs {
		if src.Kind != KindAgent {
			continue
		}
		var cfg Config
		if json.Unmarshal([]byte(src.Config), &cfg) != nil || cfg.Agent == nil {
			continue
		}
		if (agentID != 0 && cfg.Agent.AgentID == agentID) || (cfg.Agent.AgentID == 0 && agentKey(cfg.Agent.URL) == g.key) {
			m.SetStatus(ctx, src, srcErr)
		}
	}
}

// gatedFS applies the agent's gate to a file system: walks take a
// concurrency slot, every call fails fast while the agent is down, and
// network failures open the breaker.
type gatedFS struct {
	vfs.FS
	g *agentGate
}

func (f gatedFS) Walk(ctx context.Context, dir string, fn func(vfs.Entry) error) error {
	release, err := f.g.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return f.g.done(ctx, f.FS.Walk(ctx, dir, fn))
}

func (f gatedFS) Stat(ctx context.Context, name string) (vfs.Entry, error) {
	if err := f.g.check(); err != nil {
		return vfs.Entry{}, err
	}
	e, err := f.FS.Stat(ctx, name)
	return e, f.g.done(ctx, err)
}

func (f gatedFS) ReadDir(ctx context.Context, dir string) ([]vfs.Entry, error) {
	if err := f.g.check(); err != nil {
		return nil, err
	}
	es, err := f.FS.ReadDir(ctx, dir)
	return es, f.g.done(ctx, err)
}

func (f gatedFS) Open(ctx context.Context, name string) (vfs.File, error) {
	if err := f.g.check(); err != nil {
		return nil, err
	}
	file, err := f.FS.Open(ctx, name)
	if err = f.g.done(ctx, err); err != nil {
		return nil, err
	}
	return gatedFile{File: file, ctx: ctx, g: f.g}, nil
}

func (f gatedFS) Ping(ctx context.Context) error {
	if err := f.g.check(); err != nil {
		return err
	}
	return f.g.done(ctx, f.FS.Ping(ctx))
}

// gatedFile reports read failures (ranged requests) to the gate.
type gatedFile struct {
	vfs.File
	ctx context.Context
	g   *agentGate
}

func (f gatedFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = f.g.done(f.ctx, err)
	}
	return n, err
}

// ReadAt forwards bounded reads (one ranged request each) when the file
// supports them, reporting failures to the gate like Read.
func (f gatedFile) ReadAt(p []byte, off int64) (int, error) {
	ra, ok := f.File.(io.ReaderAt)
	if !ok {
		return 0, errors.ErrUnsupported
	}
	n, err := ra.ReadAt(p, off)
	if err != nil && !errors.Is(err, io.EOF) {
		err = f.g.done(f.ctx, err)
	}
	return n, err
}
