package sources

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/agent"
	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs/agentfs"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newTestManager(t *testing.T) (*Manager, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	m, err := NewForTests(st, t.TempDir(), events.NewHub(), quiet())
	if err != nil {
		t.Fatal(err)
	}
	m.ServerID, m.ServerName = "srv-test", "Test"
	return m, st
}

// testAgent runs a real agent on a loopback port with the listener Run
// uses (HTTPS and HTTP on the same port).
type testAgent struct {
	s    *agent.Server
	addr string // 127.0.0.1:port
	data string
}

func startAgent(t *testing.T, data string, mut func(*agent.Config)) *testAgent {
	t.Helper()
	share := t.TempDir()
	if err := os.WriteFile(filepath.Join(share, "a.mkv"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	srv := &http.Server{Handler: s.Handler()}
	go func() { _ = srv.Serve(s.WrapListener(ln)) }()
	t.Cleanup(func() { _ = srv.Close(); s.Close() })
	return &testAgent{s: s, addr: ln.Addr().String(), data: data}
}

func code(a *testAgent) string { c, _ := a.s.PairingCode(); return c }

func errCode(err error) string {
	var ve *ValidationError
	if errors.As(err, &ve) {
		if ve.Code == "" {
			return "invalid_source"
		}
		return ve.Code
	}
	return ""
}

func TestPairHTTPSPinned(t *testing.T) {
	m, _ := newTestManager(t)
	a := startAgent(t, t.TempDir(), nil)
	// An http:// address is upgraded to HTTPS before anything is sent.
	ag, err := m.Pair(context.Background(), "http://"+a.addr, code(a), 0)
	if err != nil {
		t.Fatal(err)
	}
	if ag.Url != "https://"+a.addr || ag.Fingerprint != a.s.Fingerprint() || AgentTransport(ag) != TransportPinned || ag.Uid != a.s.UID() {
		t.Fatalf("agent row: %+v", ag)
	}
	var info map[string]any
	if err := m.AgentCall(context.Background(), ag.ID, http.MethodGet, "/agent/v1/info", nil, &info); err != nil {
		t.Fatalf("pinned call: %v", err)
	}
	// Source on the paired agent: listed, opened, probed through the pinned
	// transport; FFmpeg gets the loopback proxy, never the agent.
	m.ProxyBase = "http://127.0.0.1:8096"
	res, err := m.Test(context.Background(), Input{Kind: KindAgent, Agent: &AgentIn{AgentID: ag.ID, Share: "films"}})
	if err != nil || !res.OK {
		t.Fatalf("test: %+v %v", res, err)
	}
	ctx := context.Background()
	libID := mustLibrary(t, m)
	src, err := m.Create(ctx, m.store.Queries, libID, Input{Kind: KindAgent, Agent: &AgentIn{AgentID: ag.ID, Share: "films"}})
	if err != nil {
		t.Fatal(err)
	}
	u, err := m.InputURL(ctx, Location(src, "a.mkv"))
	if err != nil || !strings.HasPrefix(u, "http://127.0.0.1:8096/internal/vfs/") || strings.Contains(u, a.addr) {
		t.Fatalf("FFmpeg input = %q %v", u, err)
	}
	f, err := m.Open(ctx, Location(src, "a.mkv"), 0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	_ = f.Close()
	if string(b) != "data" {
		t.Fatalf("read %q", b)
	}
	// Re-pairing (same uid) keeps a single row.
	ag2, err := m.Pair(ctx, "https://"+a.addr, code(a), ag.ID)
	if err != nil || ag2.ID != ag.ID {
		t.Fatalf("re-pair: %+v %v", ag2, err)
	}
}

func mustLibrary(t *testing.T, m *Manager) int64 {
	t.Helper()
	l, err := m.store.CreateLibrary(context.Background(), db.CreateLibraryParams{Name: "Films", Kind: "movies"})
	if err != nil {
		t.Fatal(err)
	}
	return l.ID
}

// mitm is a TLS proxy with its own certificate in front of the agent.
func mitm(t *testing.T, target string, forward bool) *httptest.Server {
	t.Helper()
	tu, _ := url.Parse("https://" + target)
	rp := httputil.NewSingleHostReverseProxy(tu)
	rp.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // the attacker does not care
	h := http.Handler(rp)
	if !forward {
		// Pretends the pairing succeeded without knowing the code.
		h = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "fake", "uid": "x", "pairVersion": 2, "proof": "AAAA"})
		})
	}
	ts := httptest.NewTLSServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func TestPairMITM(t *testing.T) {
	m, st := newTestManager(t)
	a := startAgent(t, t.TempDir(), nil)
	c := code(a)
	// Relaying interceptor: the agent sees the proof bound to another
	// certificate, refuses and throws the code away.
	px := mitm(t, a.addr, true)
	_, err := m.Pair(context.Background(), px.URL, c, 0)
	if errCode(err) != "agent_fingerprint_mismatch" {
		t.Fatalf("relay MITM: %v (%s)", err, errCode(err))
	}
	if code(a) == c {
		t.Fatal("code still valid after a binding failure")
	}
	if _, err := m.Pair(context.Background(), "https://"+a.addr, c, 0); err == nil {
		t.Fatal("burnt code accepted")
	}
	// Interceptor answering on its own: no valid agent proof.
	fake := mitm(t, a.addr, false)
	if _, err := m.Pair(context.Background(), fake.URL, code(a), 0); errCode(err) != "agent_proof_invalid" {
		t.Fatalf("fake agent: %v", err)
	}
	if n, _ := st.ListAgents(context.Background()); len(n) != 0 {
		t.Fatalf("agents saved: %+v", n)
	}
}

// switcher forwards TCP connections to a target that can change: the
// agent's address "now serves" another certificate.
type switcher struct {
	ln     net.Listener
	target atomic.Value
}

func newSwitcher(t *testing.T, target string) *switcher {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &switcher{ln: ln}
	s.target.Store(target)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				b, err := net.Dial("tcp", s.target.Load().(string))
				if err != nil {
					_ = c.Close()
					return
				}
				go func() { _, _ = io.Copy(b, c); _ = b.Close() }()
				_, _ = io.Copy(c, b)
				_ = c.Close()
			}()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func TestCertificateChangeRefused(t *testing.T) {
	m, st := newTestManager(t)
	a := startAgent(t, t.TempDir(), nil)
	sw := newSwitcher(t, a.addr)
	ag, err := m.Pair(context.Background(), "https://"+sw.ln.Addr().String(), code(a), 0)
	if err != nil {
		t.Fatal(err)
	}
	other := startAgent(t, t.TempDir(), nil) // another certificate
	sw.target.Store(other.addr)
	agentfs.ClientFor(ag.Url, ag.Fingerprint).CloseIdleConnections() // an agent restart drops them
	err = m.AgentCall(context.Background(), ag.ID, http.MethodGet, "/agent/v1/info", nil, nil)
	if agentStatus(err) != AgentCertMismatch {
		t.Fatalf("call after certificate change: %v", err)
	}
	m.pingAgents(context.Background())
	row, _ := st.GetAgent(context.Background(), ag.ID)
	if row.Status != AgentCertMismatch || !strings.Contains(row.LastError, "réappairez") {
		t.Fatalf("status %q %q", row.Status, row.LastError)
	}
	// Re-pairing with the new agent's code accepts the new certificate only
	// if it is the same agent: here it is another machine.
	if _, err := m.Pair(context.Background(), "https://"+sw.ln.Addr().String(), code(other), ag.ID); errCode(err) != "agent_not_same" {
		t.Fatalf("re-pair to another machine: %v", err)
	}
}

// TestLegacyHTTPAgent: an agent paired over HTTP before HTTPS keeps working
// over HTTP after both sides are upgraded.
func TestLegacyHTTPAgent(t *testing.T) {
	m, st := newTestManager(t)
	data := t.TempDir()
	tok := "server-token-0123456789-0123456789-abcdef"
	sum := sha256.Sum256([]byte(tok))
	old, _ := json.Marshal(map[string]any{
		"uid": "11111111-2222-4333-8444-555555555555", "name": "nas", "shares": []any{},
		"servers": []any{map[string]any{"id": "srv-test", "name": "Test", "tokenHash": hex.EncodeToString(sum[:]), "pairedAt": "2026-01-01T00:00:00Z"}},
	})
	if err := os.WriteFile(filepath.Join(data, "agent-state.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	a := startAgent(t, data, nil)
	sealed, _ := m.box.SealJSON(Secrets{Token: tok})
	ag, err := st.UpsertAgent(context.Background(), db.UpsertAgentParams{Uid: a.s.UID(), Name: "nas", Url: "http://" + a.addr, Secret: sealed})
	if err != nil {
		t.Fatal(err)
	}
	if AgentTransport(ag) != TransportHTTP {
		t.Fatal(AgentTransport(ag))
	}
	if err := m.AgentCall(context.Background(), ag.ID, http.MethodGet, "/agent/v1/info", nil, nil); err != nil {
		t.Fatalf("legacy HTTP call: %v", err)
	}
	// "Réappairer" moves it to HTTPS with a pin; HTTP is then refused.
	ag2, err := m.Pair(context.Background(), ag.Url, code(a), ag.ID)
	if err != nil || AgentTransport(ag2) != TransportPinned || ag2.ID != ag.ID {
		t.Fatalf("re-pair: %+v %v", ag2, err)
	}
	var sec Secrets
	_ = m.box.OpenJSON(ag2.Secret, &sec)
	req, _ := http.NewRequest("GET", "http://"+a.addr+"/agent/v1/info", nil)
	req.Header.Set("Authorization", "Bearer "+sec.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plain after re-pair: %d", resp.StatusCode)
	}
}

func TestManualTokenTOFU(t *testing.T) {
	m, _ := newTestManager(t)
	a := startAgent(t, t.TempDir(), nil)
	in := Input{Kind: KindAgent, Agent: &AgentIn{URL: "https://" + a.addr, Token: a.s.Token(), Share: "films"}}
	res, err := m.Test(context.Background(), in)
	if err != nil || !res.OK || res.Warning != WarnTOFU || res.Fingerprint != a.s.Fingerprint() {
		t.Fatalf("tofu: %+v %v", res, err)
	}
	src, err := m.Create(context.Background(), m.store.Queries, mustLibrary(t, m), in)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := m.Decode(src)
	if cfg.Agent.Fingerprint != a.s.Fingerprint() {
		t.Fatalf("pin not stored: %+v", cfg.Agent)
	}
	// A given, wrong fingerprint is refused.
	bad := in
	ba := *in.Agent
	ba.Fingerprint = strings.Repeat("ab", 32)
	bad.Agent = &ba
	res, _ = m.Test(context.Background(), bad)
	if res.OK || !strings.Contains(res.Error, "inattendu") {
		t.Fatalf("wrong fingerprint: %+v", res)
	}
	// The right one, in the agent's display form, is accepted silently.
	ba.Fingerprint = strings.ToLower(a.s.Fingerprint()[:8]) + " " + a.s.Fingerprint()[8:]
	res, _ = m.Test(context.Background(), bad)
	if !res.OK || res.Warning != "" {
		t.Fatalf("given fingerprint: %+v", res)
	}
}

func TestPairTLSDisabledAgent(t *testing.T) {
	m, _ := newTestManager(t)
	a := startAgent(t, t.TempDir(), func(c *agent.Config) { c.DisableTLS = true })
	ag, err := m.Pair(context.Background(), "http://"+a.addr, code(a), 0)
	if err != nil || AgentTransport(ag) != TransportHTTP {
		t.Fatalf("tls = false: %+v %v", ag, err)
	}
	// Behind an HTTPS proxy with a certificate no CA vouches for: refused
	// (the agent cannot check which certificate the server saw).
	px := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: a.addr}))
	defer px.Close()
	if _, err := m.Pair(context.Background(), px.URL, code(a), 0); errCode(err) != "agent_cert_untrusted" {
		t.Fatalf("proxy without CA: %v", err)
	}
}
