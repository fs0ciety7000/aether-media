package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agent"
	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
	"github.com/fs0ciety7000/aether-media/internal/vfs/agentfs"
)

// countingListener counts the TCP connections the agent accepts. While
// blocked, it resets every connection (open ones included), like a host
// whose anti-abuse protection blacklisted the server's address.
type countingListener struct {
	net.Listener
	accepted atomic.Int64
	blocked  atomic.Bool

	mu    sync.Mutex
	conns map[net.Conn]bool
}

func (l *countingListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		l.accepted.Add(1)
		if l.blocked.Load() {
			reset(c)
			continue
		}
		l.mu.Lock()
		l.conns[c] = true
		l.mu.Unlock()
		return &trackedConn{Conn: c, l: l}, nil
	}
}

func reset(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetLinger(0)
	}
	_ = c.Close()
}

func (l *countingListener) block(on bool) {
	l.blocked.Store(on)
	if !on {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for c := range l.conns {
		reset(c)
	}
	clear(l.conns)
}

type trackedConn struct {
	net.Conn
	l    *countingListener
	once sync.Once
}

func (c *trackedConn) Close() error {
	c.once.Do(func() { c.l.mu.Lock(); delete(c.l.conns, c.Conn); c.l.mu.Unlock() })
	return c.Conn.Close()
}

// startCountedAgent runs a real agent exporting n small .mkv files, behind
// a counting listener.
func startCountedAgent(t *testing.T, n int, mut func(*agent.Config)) (*testAgent, *countingListener) {
	t.Helper()
	share := t.TempDir()
	for i := range n {
		if err := os.WriteFile(filepath.Join(share, fmt.Sprintf("f%02d.mkv", i)), []byte("not a video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data := t.TempDir()
	cfg := agent.Config{DataDir: data, Shares: []agent.Share{{Name: "films", Path: share}}, DiscoveryPort: -1, Logger: quiet()}
	if mut != nil {
		mut(&cfg)
	}
	s, err := agent.New(cfg, "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cl := &countingListener{Listener: ln, conns: map[net.Conn]bool{}}
	srv := &http.Server{Handler: s.Handler()}
	go func() { _ = srv.Serve(s.WrapListener(cl)) }()
	t.Cleanup(func() { _ = srv.Close(); s.Close() })
	return &testAgent{s: s, addr: ln.Addr().String(), data: data}, cl
}

// pairedSource pairs the agent and creates a source on its share.
func pairedSource(t *testing.T, m *Manager, a *testAgent, scheme string) (db.RemoteAgent, db.LibrarySource) {
	t.Helper()
	ctx := context.Background()
	ag, err := m.Pair(ctx, scheme+"://"+a.addr, code(a), 0)
	if err != nil {
		t.Fatal(err)
	}
	src, err := m.Create(ctx, m.store.Queries, mustLibrary(t, m), Input{Kind: KindAgent, Agent: &AgentIn{AgentID: ag.ID, Share: "films"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { agentfs.ClientFor(ag.Url, ag.Fingerprint).CloseIdleConnections() })
	return ag, src
}

// scanAndProbe does what a library scan does against an agent: one walk,
// then an analysis of every file (with many more workers than the agent
// limit) plus the directory listing of each and a ranged read.
func scanAndProbe(t *testing.T, m *Manager, src db.LibrarySource) int {
	t.Helper()
	ctx := context.Background()
	fsys, _, err := m.FS(ctx, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	if err := fsys.Walk(ctx, "", func(e vfs.Entry) error {
		if !e.IsDir {
			files = append(files, e.Path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p := probe.New("ffprobe")
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	errs := make(chan error, len(files))
	for _, f := range files {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			loc := Location(src, f)
			if _, err := m.Probe(ctx, p, loc); !probeReached(err) {
				errs <- fmt.Errorf("probe %s: %w", f, err)
				return
			}
			if _, err := m.ReadDir(ctx, RootLocation(src)+"/"); err != nil {
				errs <- fmt.Errorf("readdir: %w", err)
				return
			}
			rf, err := m.Open(ctx, loc, 0)
			if err != nil {
				errs <- err
				return
			}
			_, _ = rf.Seek(4, io.SeekStart)
			_, err = io.ReadAll(rf)
			_ = rf.Close()
			if err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	return len(files)
}

// TestAgentConnectionReuse: a scan of N files plus N analyses opens a small,
// bounded number of connections to the agent: one over HTTPS (HTTP/2), at
// most MaxConnsPerAgent over plain HTTP (keep-alive).
func TestAgentConnectionReuse(t *testing.T) {
	const n = 30
	t.Run("https-h2", func(t *testing.T) {
		m, _ := newTestManager(t)
		a, cl := startCountedAgent(t, n, nil)
		ag, src := pairedSource(t, m, a, "https")
		if AgentTransport(ag) != TransportPinned {
			t.Fatal(AgentTransport(ag))
		}
		// Start cold: the analyses must not each dial before HTTP/2 is known.
		agentfs.ClientFor(ag.Url, ag.Fingerprint).CloseIdleConnections()
		before := cl.accepted.Load() // pairing: TLS probe + pinned connection
		if got := scanAndProbe(t, m, src); got != n {
			t.Fatalf("walked %d files", got)
		}
		opened := cl.accepted.Load() - before
		t.Logf("HTTPS : %d fichiers + %d analyses → %d connexion(s) TCP (appairage : %d)", n, n, opened, before)
		if opened > 1 {
			t.Fatalf("%d connections opened, want 1 (HTTP/2)", opened)
		}
		// HTTP/2 is negotiated over the pinned transport.
		req, _ := http.NewRequest(http.MethodGet, ag.Url+"/agent/v1/info", nil)
		res, err := agentfs.ClientFor(ag.Url, ag.Fingerprint).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		drainClose(res)
		if res.ProtoMajor != 2 {
			t.Fatalf("protocol %s, want HTTP/2", res.Proto)
		}
	})
	t.Run("http-keepalive", func(t *testing.T) {
		m, _ := newTestManager(t)
		a, cl := startCountedAgent(t, n, func(c *agent.Config) { c.DisableTLS = true })
		ag, src := pairedSource(t, m, a, "http")
		if AgentTransport(ag) != TransportHTTP {
			t.Fatal(AgentTransport(ag))
		}
		before := cl.accepted.Load()
		scanAndProbe(t, m, src)
		opened := cl.accepted.Load() - before
		t.Logf("HTTP : %d fichiers + %d analyses → %d connexion(s) TCP", n, n, opened)
		if opened > agentfs.MaxConnsPerAgent {
			t.Fatalf("%d connections opened, want ≤ %d", opened, agentfs.MaxConnsPerAgent)
		}
	})
}

// TestAgentCircuitBreaker: once the agent cannot be reached, analyses fail
// at once without touching the network (no storm of attempts), a single
// probe per backoff step checks the agent, and its answer brings it back.
func TestAgentCircuitBreaker(t *testing.T) {
	saved := agentBackoff
	agentBackoff = []time.Duration{150 * time.Millisecond, 300 * time.Millisecond}
	t.Cleanup(func() { agentBackoff = saved })

	m, st := newTestManager(t)
	t.Cleanup(m.Close)
	var online atomic.Int32
	m.OnAgentOnline = func() { online.Add(1) }
	a, cl := startCountedAgent(t, 3, nil)
	ag, src := pairedSource(t, m, a, "https")
	ctx := context.Background()
	p := probe.New("ffprobe")
	loc := Location(src, "f00.mkv")
	if _, err := m.Probe(ctx, p, loc); !probeReached(err) {
		t.Fatal(err)
	}

	// The host blocks us: established connections die, new ones are reset.
	cl.block(true)
	start := cl.accepted.Load()
	const calls = 40
	var down atomic.Int32
	var wg sync.WaitGroup
	for range calls {
		wg.Go(func() {
			_, err := m.Probe(ctx, p, loc)
			if _, ok := errors.AsType[*AgentDownError](err); ok && errors.Is(err, vfs.ErrOffline) {
				down.Add(1)
			}
		})
	}
	wg.Wait()
	if down.Load() != calls {
		t.Fatalf("%d/%d calls reported the agent down", down.Load(), calls)
	}
	if !m.AgentDown(ag.Url) {
		t.Fatal("breaker not open")
	}
	row, _ := st.GetAgent(ctx, ag.ID)
	srow, _ := st.GetSource(ctx, src.ID)
	if row.Status != "offline" || srow.Status != "offline" {
		t.Fatalf("status agent %q source %q", row.Status, srow.Status)
	}
	// Other paths fail fast too, without dialling.
	if _, err := m.Stat(ctx, loc); !errors.Is(err, vfs.ErrOffline) {
		t.Fatalf("stat while down: %v", err)
	}
	if err := m.AgentCall(ctx, ag.ID, http.MethodGet, "/agent/v1/info", nil, nil); !errors.Is(err, vfs.ErrOffline) {
		t.Fatalf("agent call while down: %v", err)
	}
	m.PingAll(ctx)
	// Stay blocked a while: only the breaker's probes reach the host.
	time.Sleep(700 * time.Millisecond)
	attempts := cl.accepted.Load() - start
	t.Logf("agent bloqué : %d appels → %d tentative(s) de connexion", calls+3, attempts)
	// The failing call(s) holding a concurrency slot, then ~1 probe per step.
	if limit := int64(DefaultAgentConcurrency + 5); attempts > limit {
		t.Fatalf("%d connection attempts while blocked, want ≤ %d", attempts, limit)
	}

	// The agent is reachable again: the next probe closes the breaker.
	cl.block(false)
	deadline := time.Now().Add(5 * time.Second)
	for m.AgentDown(ag.Url) {
		if time.Now().After(deadline) {
			t.Fatal("breaker still open after the agent came back")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if online.Load() != 1 {
		t.Fatalf("OnAgentOnline called %d times", online.Load())
	}
	row, _ = st.GetAgent(ctx, ag.ID)
	srow, _ = st.GetSource(ctx, src.ID)
	if row.Status != "online" || srow.Status != "online" {
		t.Fatalf("status after recovery: agent %q source %q", row.Status, srow.Status)
	}
	if _, err := m.Probe(ctx, p, loc); !probeReached(err) {
		t.Fatalf("probe after recovery: %v", err)
	}
}

// TestAgentConcurrencyLimit: no more than AgentConcurrency analyses run at
// once on an agent, and a busy agent does not hold up another one.
func TestAgentConcurrencyLimit(t *testing.T) {
	m, _ := newTestManager(t)
	slow := &AgentConfig{URL: "https://slow.example:1"}
	g := m.gate(*slow, Secrets{})
	other := m.gate(AgentConfig{URL: "https://other.example:1"}, Secrets{})
	ctx := context.Background()
	var rel []func()
	for range DefaultAgentConcurrency {
		r, err := g.acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rel = append(rel, r)
	}
	tctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := g.acquire(tctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slot beyond the limit: %v", err)
	}
	r, err := other.acquire(ctx)
	if err != nil {
		t.Fatalf("other agent blocked: %v", err)
	}
	r()
	for _, r := range rel {
		r()
	}
}

// probeReached: the probe request made the round trip to the agent. These
// tests count connections, not probe results, so an agent without ffprobe
// (CI runners) is fine.
func probeReached(err error) bool {
	if err == nil || errors.Is(err, probe.ErrNotMedia) {
		return true
	}
	ae, ok := errors.AsType[*AgentError](err)
	return ok && ae.Code == "ffprobe_unavailable"
}
