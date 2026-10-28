package agent

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agentproto"
)

func TestSelfSignedCertificate(t *testing.T) {
	data := t.TempDir()
	s1, err := New(Config{DataDir: data, Logger: quietLogger(), DiscoveryPort: -1}, "v")
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()
	fp := s1.Fingerprint()
	if len(fp) != 64 || s1.tls.source != certSelfSigned {
		t.Fatalf("fingerprint %q source %q", fp, s1.tls.source)
	}
	b, err := os.ReadFile(filepath.Join(data, certFileName))
	if err != nil {
		t.Fatal(err)
	}
	blk, _ := pem.Decode(b)
	c, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if agentproto.CertFingerprint(c) != fp || c.PublicKeyAlgorithm != x509.ECDSA ||
		c.NotAfter.Before(time.Now().Add(15*365*24*time.Hour)) || c.VerifyHostname("localhost") != nil || c.VerifyHostname("127.0.0.1") != nil {
		t.Fatalf("certificate: %v %v %v", c.PublicKeyAlgorithm, c.NotAfter, c.DNSNames)
	}
	// Stable across restarts; permissions are repaired.
	_ = os.Chmod(filepath.Join(data, keyFileName), 0o644)
	s2, err := New(Config{DataDir: data, Logger: quietLogger(), DiscoveryPort: -1}, "v")
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
	if s2.Fingerprint() != fp {
		t.Fatal("certificate regenerated")
	}
	if fi, _ := os.Stat(filepath.Join(data, keyFileName)); fi.Mode().Perm() != 0o600 {
		t.Fatal("key permissions not fixed")
	}
	// A key without certificate is not silently replaced.
	_ = os.Remove(filepath.Join(data, certFileName))
	if _, err := New(Config{DataDir: data, Logger: quietLogger()}, "v"); err == nil {
		t.Fatal("orphan key accepted")
	}

	// tls_cert / tls_key: their fingerprint, origin "custom".
	cert, key, err := selfSigned("nas", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cp, kp := filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	_ = os.WriteFile(cp, cert, 0o600)
	_ = os.WriteFile(kp, key, 0o600)
	s3, err := New(Config{DataDir: t.TempDir(), TLSCert: cp, TLSKey: kp, Logger: quietLogger()}, "v")
	if err != nil {
		t.Fatal(err)
	}
	s3.Close()
	blk, _ = pem.Decode(cert)
	if s3.Fingerprint() != agentproto.Fingerprint(blk.Bytes) || s3.tls.source != certCustom {
		t.Fatal("custom certificate")
	}
	// tls = false: no certificate at all.
	d4 := t.TempDir()
	s4, err := New(Config{DataDir: d4, DisableTLS: true, Logger: quietLogger()}, "v")
	if err != nil {
		t.Fatal(err)
	}
	s4.Close()
	if s4.Fingerprint() != "" || s4.TLSConfig() != nil {
		t.Fatal("tls = false still serves TLS")
	}
	if _, err := os.Stat(filepath.Join(d4, certFileName)); err == nil {
		t.Fatal("certificate created with tls = false")
	}
	if _, err := New(Config{DataDir: t.TempDir(), DisableTLS: true, TLSCert: cp, TLSKey: kp, Logger: quietLogger()}, "v"); err == nil {
		t.Fatal("tls = false with tls_cert accepted")
	}
}

func TestTLSConfigKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.toml")
	writeFile(t, p, "tls = false\nhttp_compat = false\n")
	t.Setenv("AETHER_AGENT_TLS", "")
	cfg, err := LoadConfig(p)
	if err != nil || !cfg.DisableTLS || !cfg.DisableHTTPCompat {
		t.Fatalf("%+v %v", cfg, err)
	}
	t.Setenv("AETHER_AGENT_TLS", "1")
	if cfg, err := LoadConfig(p); err != nil || cfg.DisableTLS {
		t.Fatalf("env override: %+v %v", cfg, err)
	}
	t.Setenv("AETHER_AGENT_TLS", "0")
	if cfg, err := LoadConfig(""); err != nil || !cfg.DisableTLS {
		t.Fatalf("AETHER_AGENT_TLS=0: %+v %v", cfg, err)
	}
	t.Setenv("AETHER_AGENT_TLS", "peut-être")
	if _, err := LoadConfig(""); err == nil {
		t.Fatal("bad AETHER_AGENT_TLS accepted")
	}
}

func TestPairingBinding(t *testing.T) {
	e := newEnv(t, nil)
	code, _ := e.s.PairingCode()
	// A server that saw another certificate (interception): refused, and
	// the code is thrown away even though it was right.
	other := strings.Repeat("ab", 32)
	st, _, ec := e.pairFP(code, "srv", other)
	if st != http.StatusConflict || ec != "fingerprint_mismatch" {
		t.Fatalf("mismatch: %d %s", st, ec)
	}
	if c2, _ := e.s.PairingCode(); c2 == code {
		t.Fatal("code not invalidated after a binding failure")
	}
	if st, _, _ := e.pairFP(code, "srv", e.s.Fingerprint()); st != 401 {
		t.Fatalf("invalidated code reused: %d", st)
	}
	// A wrong code bound to the right certificate: 401, code kept.
	c3, _ := e.s.PairingCode()
	if st, _, ec := e.pairFP("ZZZZ-ZZZZ", "srv", e.s.Fingerprint()); st != 401 || ec != "invalid_code" {
		t.Fatalf("wrong code: %d %s", st, ec)
	}
	if c4, _ := e.s.PairingCode(); c4 != c3 {
		t.Fatal("wrong code rotated the code")
	}
	// The proof is bound to the server id too.
	r := e.postJSON("POST", "/pair", "", func() pairRequest {
		nonce := make([]byte, agentproto.NonceLen)
		key := agentproto.PairKey(normalizePairCode(c3), nonce)
		return pairRequest{Version: 2, ServerID: "mallory", Fingerprint: e.s.Fingerprint(),
			Nonce: b64(nonce), Proof: b64(agentproto.ServerProof(key, e.s.Fingerprint(), nonce, "srv"))}
	}())
	if r.StatusCode != 401 {
		t.Fatalf("proof for another server id: %d", r.StatusCode)
	}
	// v1 (code in clear) is refused and burns the code.
	c5, _ := e.s.PairingCode()
	r = e.postJSON("POST", "/pair", "", map[string]string{"code": c5, "serverId": "old"})
	if r.StatusCode != 400 || errCode(t, r) != "pairing_upgrade_required" {
		t.Fatalf("v1: %d", r.StatusCode)
	}
	if c6, _ := e.s.PairingCode(); c6 == c5 {
		t.Fatal("code sent in clear not invalidated")
	}
	// Plain HTTP pairing on an HTTPS agent.
	c7, _ := e.s.PairingCode()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", apiPrefix+"/pair", strings.NewReader(`{"version":2,"proof":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	e.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("plain pairing: %d", rec.Code)
	}
	if st, pr := e.pair(c7, "srv"); st != 200 || pr.Fingerprint != e.s.Fingerprint() || pr.CertSource != certSelfSigned || pr.PairVersion != 2 {
		t.Fatalf("pair: %d %+v", st, pr)
	}
	if e.s.state.Servers[0].Plain {
		t.Fatal("server paired over HTTPS marked plain")
	}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// plainReq runs an authenticated request over plain HTTP against the
// handler (as the sniffing listener would deliver it).
func plainReq(s *Server, path, token string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	s.Handler().ServeHTTP(rec, req)
	return rec.Code
}

// TestUpgradeFromHTTP: an agent state written before HTTPS keeps its
// pairings working over HTTP until they pair again.
func TestUpgradeFromHTTP(t *testing.T) {
	data := t.TempDir()
	tok := "legacy-token-0123456789-0123456789-abcdef"
	writeFile(t, filepath.Join(data, tokenFileName), tok+"\n")
	srvTok := "server-token-0123456789-0123456789-abcdef"
	sum := sha256.Sum256([]byte(srvTok))
	old := map[string]any{
		"uid": "11111111-2222-4333-8444-555555555555", "name": "nas", "shares": []any{},
		"servers": []any{map[string]any{"id": "srv-old", "name": "Salon", "tokenHash": hex.EncodeToString(sum[:]), "pairedAt": "2026-01-01T00:00:00Z"}},
	}
	b, _ := json.Marshal(old)
	writeFile(t, filepath.Join(data, stateFileName), string(b))
	s, err := New(Config{DataDir: data, Logger: quietLogger(), DiscoveryPort: -1}, "v")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.state.Version != stateVersion || !s.state.Servers[0].Plain || !s.state.LegacyTokenPlain {
		t.Fatalf("upgrade: %+v", s.state)
	}
	if st := plainReq(s, apiPrefix+"/info", srvTok); st != 200 {
		t.Fatalf("server paired over HTTP, plain: %d", st)
	}
	if st := plainReq(s, apiPrefix+"/info", tok); st != 200 {
		t.Fatalf("legacy token, plain: %d", st)
	}
	ls := s.localStatus()
	if !ls.HTTPCompat || !ls.Servers[0].Plain || ls.Fingerprint == "" {
		t.Fatalf("local status: %+v", ls)
	}
	// Re-pairing over HTTPS ends the exception for that server.
	ts := startTestServer(s)
	defer ts.Close()
	e := &env{t: t, s: s, ts: ts}
	code, _ := s.PairingCode()
	st, pr := e.pair(code, "srv-old")
	if st != 200 {
		t.Fatal(st)
	}
	if st := plainReq(s, apiPrefix+"/info", pr.Token); st != http.StatusUpgradeRequired {
		t.Fatalf("re-paired server, plain: %d", st)
	}
	if st := plainReq(s, apiPrefix+"/info", srvTok); st != 401 {
		t.Fatalf("old token after re-pair: %d", st)
	}
	// The state was saved upgraded: a restart does not mark it plain again.
	s.Close()
	s2, err := New(Config{DataDir: data, Logger: quietLogger(), DiscoveryPort: -1}, "v")
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.state.Servers[0].Plain || !s2.state.LegacyTokenPlain {
		t.Fatalf("after restart: %+v", s2.state)
	}
	// http_compat = false refuses plain HTTP entirely.
	s2.Close()
	s3, err := New(Config{DataDir: data, DisableHTTPCompat: true, Logger: quietLogger(), DiscoveryPort: -1}, "v")
	if err != nil {
		t.Fatal(err)
	}
	defer s3.Close()
	if st := plainReq(s3, apiPrefix+"/info", tok); st != http.StatusUpgradeRequired {
		t.Fatalf("http_compat = false: %d", st)
	}
	// A regenerated agent.token is never accepted over HTTP.
	s3.Close()
	_ = os.Remove(filepath.Join(data, tokenFileName))
	s4, err := New(Config{DataDir: data, Logger: quietLogger(), DiscoveryPort: -1}, "v")
	if err != nil {
		t.Fatal(err)
	}
	defer s4.Close()
	if s4.state.LegacyTokenPlain || plainReq(s4, apiPrefix+"/info", s4.Token()) != http.StatusUpgradeRequired {
		t.Fatal("new agent.token accepted over HTTP")
	}
}

func TestFreshStateNoCompat(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), Logger: quietLogger()}, "v")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.state.LegacyTokenPlain || s.state.Version != stateVersion || plainReq(s, apiPrefix+"/info", s.Token()) != http.StatusUpgradeRequired {
		t.Fatalf("fresh agent: %+v", s.state)
	}
	// v1 agent (agent.token, no state file): the token keeps working.
	data := t.TempDir()
	writeFile(t, filepath.Join(data, tokenFileName), "v1-token-0123456789-0123456789-0123456789\n")
	s2, err := New(Config{DataDir: data, Logger: quietLogger()}, "v")
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if !s2.state.LegacyTokenPlain || plainReq(s2, apiPrefix+"/info", s2.Token()) != 200 {
		t.Fatal("v1 agent.token refused over HTTP")
	}
}

// TestPairingWithoutTLS: tls = false (behind a proxy): the proof is bound
// to whatever certificate the server saw, the pairing is marked plain.
func TestPairingWithoutTLS(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DisableTLS = true })
	if !strings.HasPrefix(e.ts.URL, "http://") {
		t.Fatal(e.ts.URL)
	}
	code, _ := e.s.PairingCode()
	proxyFP := strings.Repeat("cd", 32)
	st, pr, _ := e.pairFP(code, "srv", proxyFP)
	if st != 200 || pr.Fingerprint != "" || pr.CertSource != "" {
		t.Fatalf("pair: %d %+v", st, pr)
	}
	if st, _ := e.status("GET", e.url("/info", nil), pr.Token); st != 200 {
		t.Fatalf("info: %d", st)
	}
	if !e.s.state.Servers[0].Plain || e.s.localStatus().Servers[0].Plain {
		t.Fatal("plain flag")
	}
}
