package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agentproto"
)

// postJSON sends a JSON body.
func (e *env) postJSON(method, endpoint, token string, body any) result {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(method, e.url(endpoint, nil), bytes.NewReader(b))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return result{StatusCode: resp.StatusCode, Header: resp.Header, Body: &buf}
}

func (e *env) pair(code, id string) (int, pairResponse) {
	e.t.Helper()
	st, pr, _ := e.pairFP(code, id, e.s.Fingerprint())
	return st, pr
}

// pairFP runs a v2 pairing as a server that saw certificate fp; it checks
// the agent's proof on success and returns the error code otherwise.
func (e *env) pairFP(code, id, fp string) (int, pairResponse, string) {
	e.t.Helper()
	nonce := make([]byte, agentproto.NonceLen)
	_, _ = rand.Read(nonce)
	key := agentproto.PairKey(normalizePairCode(code), nonce)
	r := e.postJSON("POST", "/pair", "", pairRequest{
		Version: agentproto.PairVersion, ServerName: "Salon", ServerID: id, Fingerprint: fp,
		Nonce: base64.RawURLEncoding.EncodeToString(nonce),
		Proof: base64.RawURLEncoding.EncodeToString(agentproto.ServerProof(key, fp, nonce, id)),
	})
	var pr pairResponse
	if r.StatusCode != 200 {
		var ae apiError
		_ = json.NewDecoder(r.Body).Decode(&ae)
		return r.StatusCode, pr, ae.Error
	}
	if err := json.NewDecoder(r.Body).Decode(&pr); err != nil {
		e.t.Fatal(err)
	}
	proof, _ := base64.RawURLEncoding.DecodeString(pr.Proof)
	if !agentproto.Equal(proof, agentproto.AgentProof(key, fp, nonce, pr.Token)) {
		e.t.Fatal("agent proof does not verify")
	}
	return 200, pr, ""
}

func TestZeroConfig(t *testing.T) {
	t.Setenv("AETHER_AGENT_LISTEN", "")
	t.Setenv("AETHER_AGENT_DATA_DIR", "")
	cfg, err := LoadConfig("")
	if err != nil || cfg.Listen != DefaultListen || cfg.DiscoveryPort != DefaultDiscoveryPort || len(cfg.Shares) != 0 {
		t.Fatalf("zero config: %+v %v", cfg, err)
	}
	cfg.Listen = "127.0.0.1:0"
	cfg.DiscoveryPort = -1
	cfg.DataDir = t.TempDir()
	cfg.Logger = quietLogger()
	s, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	if s.UID() == "" || s.Name() == "" {
		t.Fatalf("uid %q name %q", s.UID(), s.Name())
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	var port int32
	for range 100 {
		if port = s.httpPort.Load(); port > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// HTTPS by default, with the self-signed certificate of the data dir.
	base := "https://127.0.0.1:" + strconv.Itoa(int(port))
	var seen string
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // the test checks the fingerprint below
		VerifyConnection: func(cs tls.ConnectionState) error {
			seen = agentproto.CertFingerprint(cs.PeerCertificates[0])
			return nil
		},
	}}}
	req, _ := http.NewRequest("GET", base+apiPrefix+"/info", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var info infoResponse
	_ = json.NewDecoder(resp.Body).Decode(&info)
	resp.Body.Close()
	if resp.StatusCode != 200 || info.UID != s.UID() || info.Name != s.Name() || len(info.Shares) != 0 || info.Shares == nil ||
		!info.TLS || info.Fingerprint != s.Fingerprint() || seen != s.Fingerprint() || len(seen) != 64 {
		t.Fatalf("info: %d %+v (seen %s)", resp.StatusCode, info, seen)
	}
	for _, f := range []string{certFileName, keyFileName} {
		if fi, err := os.Stat(filepath.Join(cfg.DataDir, f)); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v", f, err)
		}
	}
	// Same port, plain HTTP: the local page works, the API demands HTTPS
	// (a fresh agent has no pre-HTTPS pairing).
	plain := "http://127.0.0.1:" + strconv.Itoa(int(port))
	resp, err = http.Get(plain + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(page), agentproto.FormatFingerprint(seen)) {
		t.Fatalf("local page over loopback: %d", resp.StatusCode)
	}
	req, _ = http.NewRequest("GET", plain+apiPrefix+"/info", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token())
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plain API on a fresh HTTPS agent: %d", resp.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not stop")
	}
	if fi, err := os.Stat(filepath.Join(cfg.DataDir, stateFileName)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state file: %v", err)
	}
}

func sha256Sum(tok string) [sha256.Size]byte { return sha256.Sum256([]byte(tok)) }

func TestPairing(t *testing.T) {
	e := newEnv(t, nil)
	writeFile(t, filepath.Join(e.share, "a.mkv"), "x")
	code, exp := e.s.PairingCode()
	if len(code) != 9 || code[4] != '-' || time.Until(exp) > pairCodeTTL || time.Until(exp) < pairCodeTTL-time.Minute {
		t.Fatalf("code %q exp %v", code, exp)
	}
	for _, c := range strings.ReplaceAll(code, "-", "") {
		if !strings.ContainsRune(pairAlphabet, c) {
			t.Fatalf("bad char %q", c)
		}
	}
	// Wrong content type is refused without consuming the code.
	req, _ := http.NewRequest("POST", e.url("/pair", nil), strings.NewReader(`{"version":2}`))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 415 {
		t.Fatalf("text/plain: %d", resp.StatusCode)
	}

	st, pr := e.pair(strings.ToLower(strings.ReplaceAll(code, "-", "")), "srv-1")
	if st != 200 || len(pr.Token) != 43 || pr.UID != e.s.UID() || pr.Name != e.s.Name() || pr.Version != "1.2.3" {
		t.Fatalf("pair: %d %+v", st, pr)
	}
	r := e.do("GET", e.url("/walk", q("share", "films")), pr.Token, nil)
	if r.StatusCode != 200 || !strings.Contains(readAll(t, r.Body), "a.mkv") {
		t.Fatalf("walk with paired token: %d", r.StatusCode)
	}
	if st, _ := e.pair(code, "srv-2"); st != 401 {
		t.Fatalf("code reused: %d", st)
	}
	if code2, _ := e.s.PairingCode(); code2 == code {
		t.Fatal("code not rotated after pairing")
	}
	if st, _ := e.status("GET", e.url("/info", nil), e.s.Token()); st != 200 {
		t.Fatalf("legacy token: %d", st)
	}
	if st, code := e.status("DELETE", e.url("/pair", nil), e.s.Token()); st != 409 || code != "legacy_token" {
		t.Fatalf("unpair legacy: %d %s", st, code)
	}

	// Pairing the same server again replaces its token.
	c3, _ := e.s.PairingCode()
	st, pr2 := e.pair(c3, "srv-1")
	if st != 200 {
		t.Fatal(st)
	}
	if st, _ := e.status("GET", e.url("/info", nil), pr.Token); st != 401 {
		t.Fatalf("old token after re-pair: %d", st)
	}
	if st, _ := e.status("DELETE", e.url("/pair", nil), pr2.Token); st != 204 {
		t.Fatalf("unpair: %d", st)
	}
	if st, _ := e.status("GET", e.url("/info", nil), pr2.Token); st != 401 {
		t.Fatalf("revoked token: %d", st)
	}
	if len(e.s.state.Servers) != 0 {
		t.Fatalf("servers left: %+v", e.s.state.Servers)
	}
}

func TestPairingRotation(t *testing.T) {
	e := newEnv(t, nil)
	now := time.Now()
	e.s.pairing.now = func() time.Time { return now }
	c1, _ := e.s.PairingCode()
	now = now.Add(9 * time.Minute)
	if c, _ := e.s.PairingCode(); c != c1 {
		t.Fatal("rotated too early")
	}
	now = now.Add(time.Minute)
	c2, exp := e.s.PairingCode()
	if c2 == c1 || !exp.Equal(now.Add(pairCodeTTL)) {
		t.Fatalf("not rotated: %s %s", c1, c2)
	}
	if st, _ := e.pair(c1, "x"); st != 401 {
		t.Fatalf("expired code accepted: %d", st)
	}
	if st, _ := e.pair(c2, "x"); st != 200 {
		t.Fatalf("current code: %d", st)
	}
}

func TestPairingRateLimit(t *testing.T) {
	e := newEnv(t, nil)
	now := time.Now()
	e.s.pairLimiter.now = func() time.Time { return now }
	for i := range pairMaxFails {
		if st, _ := e.pair("AAAA-AAAA", "x"); st != 401 {
			t.Fatalf("attempt %d: %d", i, st)
		}
	}
	code, _ := e.s.PairingCode()
	r := e.postJSON("POST", "/pair", "", pairRequest{Version: 2, Proof: "x", ServerID: "x"})
	if r.StatusCode != 429 || r.Header.Get("Retry-After") == "" {
		t.Fatalf("expected 429, got %d", r.StatusCode)
	}
	// The token API keeps working (separate limiter).
	if st, _ := e.status("GET", e.url("/info", nil), e.s.Token()); st != 200 {
		t.Fatalf("info: %d", st)
	}
	now = now.Add(pairFailSpan)
	if st, _ := e.pair(code, "x"); st != 200 {
		t.Fatalf("after window: %d", st)
	}
	if st, _ := e.pair("", "bad id with spaces"); st != 400 {
		t.Fatalf("bad server id: %d", st)
	}
}

func TestLimiterKey(t *testing.T) {
	if limiterKey("2001:db8:1:2:aaaa::1") != limiterKey("2001:db8:1:2:bbbb::2") {
		t.Fatal("IPv6 /64 not grouped")
	}
	if limiterKey("192.0.2.1") != "192.0.2.1" {
		t.Fatal("IPv4 changed")
	}
}

func TestStatePersistence(t *testing.T) {
	roots := t.TempDir()
	media := filepath.Join(roots, "Mes Films")
	if err := os.Mkdir(media, 0o755); err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	cfg := Config{DataDir: data, BrowseRoots: []string{roots}, Logger: quietLogger(), DiscoveryPort: -1}
	s1, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	sc, created, err := s1.AddShare(media, "")
	if err != nil || !created || sc.Name != "mes-films" {
		t.Fatalf("add: %+v %v %v", sc, created, err)
	}
	ts := startTestServer(s1)
	e := &env{t: t, s: s1, ts: ts}
	code, _ := s1.PairingCode()
	st, pr := e.pair(code, "srv")
	ts.Close()
	s1.Close()
	if st != 200 {
		t.Fatal(st)
	}
	p := filepath.Join(data, stateFileName)
	fi, err := os.Stat(p)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("perms: %v %v", fi, err)
	}
	var raw map[string]any
	b, _ := os.ReadFile(p)
	if err := json.Unmarshal(b, &raw); err != nil || raw["uid"] != s1.UID() || strings.Contains(string(b), pr.Token) {
		t.Fatalf("state: %s", b)
	}
	srv := raw["servers"].([]any)[0].(map[string]any)
	if srv["id"] != "srv" || srv["name"] != "Salon" || len(srv["tokenHash"].(string)) != 64 || srv["pairedAt"] == "" {
		t.Fatalf("server entry: %v", srv)
	}

	_ = os.Chmod(p, 0o644)
	s2, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.UID() != s1.UID() {
		t.Fatal("uid changed")
	}
	if sh := s2.lookupShare("mes-films"); sh == nil || !sh.managed {
		t.Fatal("managed share lost")
	}
	if c, ok := s2.matchToken(sha256Sum(pr.Token)); !ok || c.serverID != "srv" {
		t.Fatal("paired token lost")
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatal("perms not fixed")
	}

	// An unavailable managed share does not prevent startup and is kept.
	s2.Close()
	if err := os.Rename(media, media+".off"); err != nil {
		t.Fatal(err)
	}
	s3, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	infos := s3.shareInfos()
	s3.Close()
	if len(infos) != 1 || !infos[0].Unavailable {
		t.Fatalf("unavailable share: %+v", infos)
	}
	_ = os.Rename(media+".off", media)
	s4, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	s4.Close()
	if s4.lookupShare("mes-films") == nil {
		t.Fatal("share not back")
	}

	// A corrupt state file is an error.
	_ = os.WriteFile(p, []byte("{nope"), 0o600)
	if _, err := New(cfg, "v"); err == nil {
		t.Fatal("corrupt state accepted")
	}
}

func TestConfigName(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), Name: "Grenier", Logger: quietLogger()}, "v")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s.Name() != "Grenier" {
		t.Fatal(s.Name())
	}
}

func browseEnv(t *testing.T) (*env, string) {
	roots := t.TempDir()
	roots, _ = filepath.EvalSymlinks(roots)
	return newEnv(t, func(c *Config) { c.BrowseRoots = []string{roots} }), roots
}

func TestBrowse(t *testing.T) {
	e, root := browseEnv(t)
	for _, d := range []string{"Films/Action", "Séries", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, "f.txt"), "")
	get := func(p string) (int, browseResponse) {
		r := e.get("/browse", q("path", p))
		var br browseResponse
		if r.StatusCode == 200 {
			if err := json.NewDecoder(r.Body).Decode(&br); err != nil {
				t.Fatal(err)
			}
		}
		return r.StatusCode, br
	}
	st, br := get("")
	if st != 200 || br.Path != "" || len(br.Dirs) != 1 || br.Dirs[0].Path != root {
		t.Fatalf("roots: %d %+v", st, br)
	}
	st, br = get(root)
	if st != 200 || br.Path != root || br.Parent != "" || len(br.Dirs) != 2 || br.Dirs[0].Name != "Films" {
		t.Fatalf("root: %d %+v", st, br)
	}
	st, br = get(filepath.Join(root, "Films"))
	if st != 200 || br.Parent != root || len(br.Dirs) != 1 || br.Dirs[0].Path != filepath.Join(root, "Films", "Action") {
		t.Fatalf("sub: %d %+v", st, br)
	}
	if st, _ := get(filepath.Join(root, "Films", "Action")); st != 200 {
		t.Fatal(st)
	}
	r := e.get("/browse", q("path", filepath.Dir(root)))
	if r.StatusCode != 403 || errCode(t, r) != "denied" {
		t.Fatalf("outside: %d", r.StatusCode)
	}
	if st, _ := get(filepath.Join(root, "..")); st != 403 {
		t.Fatalf("dotdot: %d", st)
	}
	if st, _ := get("relative"); st != 400 {
		t.Fatalf("relative: %d", st)
	}
	if st, _ := get(filepath.Join(root, "f.txt")); st != 400 {
		t.Fatalf("file: %d", st)
	}
	if st, _ := e.status("GET", e.url("/browse", nil), ""); st != 401 {
		t.Fatalf("unauthenticated: %d", st)
	}
}

func TestDeriveShareName(t *testing.T) {
	cases := map[string]string{
		"Films":                   "films",
		"Mes Séries TV":           "mes-series-tv",
		"Œuvres  d'art!!":         "oeuvres-d-art",
		"__x__":                   "__x__",
		"日本":                      "partage",
		"...":                     "partage",
		strings.Repeat("a", 60):   strings.Repeat("a", 32),
		"Kids (2020) - Ça tourne": "kids-2020-ca-tourne",
	}
	for in, want := range cases {
		if got := deriveShareName(in); got != want || !validShareName(got) {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	taken := map[string]bool{"films": true, "films-2": true}
	if got := uniqueName("films", func(n string) bool { return taken[n] }); got != "films-3" {
		t.Fatal(got)
	}
}

func TestSharesAPI(t *testing.T) {
	e, root := browseEnv(t)
	a := filepath.Join(root, "a", "Films")
	b := filepath.Join(root, "b", "Films")
	for _, d := range []string{a, b, filepath.Join(root, "c")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(a, "heat.mkv"), "h")
	tok := e.s.Token()
	add := func(body map[string]string) (int, Share, string) {
		r := e.postJSON("POST", "/shares", tok, body)
		var sc Share
		var ae apiError
		raw := readAll(t, r.Body)
		_ = json.Unmarshal([]byte(raw), &sc)
		_ = json.Unmarshal([]byte(raw), &ae)
		return r.StatusCode, sc, ae.Error
	}
	// "films" is taken by the toml share: derived names are suffixed.
	st, sc, _ := add(map[string]string{"path": a})
	if st != 201 || sc.Name != "films-2" || sc.Path != a {
		t.Fatalf("add a: %d %+v", st, sc)
	}
	if st, sc2, _ := add(map[string]string{"path": a + "/"}); st != 200 || sc2.Name != "films-2" {
		t.Fatalf("idempotent: %d %+v", st, sc2)
	}
	if st, sc3, _ := add(map[string]string{"path": b}); st != 201 || sc3.Name != "films-3" {
		t.Fatalf("dedupe: %d %+v", st, sc3)
	}
	if st, _, code := add(map[string]string{"path": filepath.Join(root, "c"), "name": "films"}); st != 409 || code != "name_taken" {
		t.Fatalf("taken: %d %s", st, code)
	}
	if st, _, code := add(map[string]string{"path": filepath.Join(root, "c"), "name": "Bad Name"}); st != 400 || code != "invalid_name" {
		t.Fatalf("invalid name: %d %s", st, code)
	}
	if st, sc4, _ := add(map[string]string{"path": filepath.Join(root, "c"), "name": "kids"}); st != 201 || sc4.Name != "kids" {
		t.Fatalf("explicit name: %d %+v", st, sc4)
	}
	if st, _, code := add(map[string]string{"path": e.share}); st != 200 {
		t.Fatalf("toml share path: %d %s", st, code)
	}
	if st, _, code := add(map[string]string{"path": t.TempDir()}); st != 403 || code != "denied" {
		t.Fatalf("outside roots: %d %s", st, code)
	}
	if st, _, _ := add(map[string]string{"path": filepath.Join(root, "missing")}); st != 403 {
		t.Fatalf("missing: %d", st)
	}
	if st, _, _ := add(map[string]string{"path": "rel"}); st != 400 {
		t.Fatalf("relative: %d", st)
	}

	// Immediately served.
	r := e.get("/walk", q("share", "films-2"))
	if r.StatusCode != 200 || !strings.Contains(readAll(t, r.Body), "heat.mkv") {
		t.Fatalf("walk new share: %d", r.StatusCode)
	}
	var list []shareInfo
	if err := json.NewDecoder(e.get("/shares", nil).Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 || list[0].Name != "films" || list[0].Managed || !list[1].Managed || list[1].Path != a {
		t.Fatalf("list: %+v", list)
	}

	if st, code := e.status("DELETE", e.url("/shares/films", nil), tok); st != 409 || code != "not_managed" {
		t.Fatalf("delete toml: %d %s", st, code)
	}
	if st, _ := e.status("DELETE", e.url("/shares/nope", nil), tok); st != 404 {
		t.Fatalf("delete unknown: %d", st)
	}
	old := e.s.lookupShare("films-2")
	if st, _ := e.status("DELETE", e.url("/shares/films-2", nil), tok); st != 204 {
		t.Fatalf("delete: %d", st)
	}
	if r := e.get("/walk", q("share", "films-2")); r.StatusCode != 404 {
		t.Fatalf("walk deleted: %d", r.StatusCode)
	}
	// An in-flight request still holding the share fails cleanly.
	if _, err := old.root.Stat("heat.mkv"); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("root not closed: %v", err)
	}
	rec := httptest.NewRecorder()
	e.s.fsError(rec, &fs.PathError{Op: "stat", Path: "x", Err: os.ErrClosed})
	if rec.Code != 404 {
		t.Fatalf("closed root → %d", rec.Code)
	}
	if !old.removed.Load() {
		t.Fatal("removed flag")
	}
	// State reflects the deletion.
	st2, _, _ := loadState(e.data)
	if len(st2.Shares) != 2 || st2.Shares[0].Name != "films-3" {
		t.Fatalf("state: %+v", st2.Shares)
	}
}

func TestAddedShareWatched(t *testing.T) {
	e, root := browseEnv(t)
	e.s.debounce = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		_ = e.s.watch(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-watchDone
		e.s.hub.close()
	})
	for range 100 { // wait for the watcher to be published
		e.s.watchMu.Lock()
		w := e.s.watcher
		e.s.watchMu.Unlock()
		if w != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", e.url("/events", nil), nil)
	req.Header.Set("Authorization", "Bearer "+e.s.Token())
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	ch := make(chan sseEvent, 100)
	go readSSE(resp.Body, ch)

	dir := filepath.Join(root, "Musique")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := e.postJSON("POST", "/shares", e.s.Token(), map[string]string{"path": dir}); r.StatusCode != 201 {
		t.Fatalf("add: %d", r.StatusCode)
	}
	writeFile(t, filepath.Join(dir, "sub", "song.flac"), "x")
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.name != "change" {
				continue
			}
			var ce changeEvent
			_ = json.Unmarshal([]byte(ev.data), &ce)
			if ce.Share == "musique" && len(ce.Paths) > 0 && strings.HasPrefix(ce.Paths[0], "sub") {
				goto removed
			}
		case <-deadline:
			t.Fatal("no event for the added share")
		}
	}
removed:
	if err := e.s.RemoveShare("musique"); err != nil {
		t.Fatal(err)
	}
	e.s.watchMu.Lock()
	for _, p := range e.s.watcher.WatchList() {
		if p == dir || within(p, dir) {
			t.Errorf("still watched: %s", p)
		}
	}
	e.s.watchMu.Unlock()
}

func TestLocalPage(t *testing.T) {
	e := newEnv(t, nil)
	h := e.s.Handler()
	serve := func(path, remote, host string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = remote
		req.Host = host
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	code, _ := e.s.PairingCode()
	rec := serve("/", "127.0.0.1:5000", "localhost:8097", nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") ||
		!strings.Contains(body, code) || !strings.Contains(body, `http-equiv="refresh" content="30"`) ||
		!strings.Contains(body, "films") || !strings.Contains(body, "#04121D") || rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("page: %d\n%s", rec.Code, body)
	}
	if rec := serve("/", "[::1]:5000", "[::1]:8097", nil); rec.Code != 200 {
		t.Fatalf("ipv6 loopback: %d", rec.Code)
	}
	if rec := serve("/", "127.0.0.1:5000", "127.0.0.1", nil); rec.Code != 200 {
		t.Fatalf("127.0.0.1 host: %d", rec.Code)
	}
	for name, c := range map[string][3]string{
		"lan":       {"192.168.1.5:4000", "192.168.1.2:8097", ""},
		"rebinding": {"127.0.0.1:4000", "evil.example:8097", ""},
		"proxied":   {"127.0.0.1:4000", "localhost:8097", "1"},
	} {
		hdr := map[string]string{}
		if c[2] != "" {
			hdr["X-Forwarded-For"] = "203.0.113.9"
		}
		for _, p := range []string{"/", apiPrefix + "/local/status"} {
			if rec := serve(p, c[0], c[1], hdr); rec.Code != 404 || strings.Contains(rec.Body.String(), code) {
				t.Errorf("%s %s: %d", name, p, rec.Code)
			}
		}
	}
	rec = serve(apiPrefix+"/local/status", "127.0.0.1:5000", "localhost", nil)
	var st localStatus
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil || st.PairingCode != code || st.UID != e.s.UID() ||
		len(st.Shares) != 1 || st.Version != "1.2.3" || st.Servers == nil {
		t.Fatalf("status: %+v %v", st, err)
	}
	for _, u := range st.Addresses {
		if pu, err := url.Parse(u); err != nil || pu.Port() != "8097" || net.ParseIP(pu.Hostname()).IsLoopback() {
			t.Errorf("address %q", u)
		}
	}
	// Other unknown paths still require authentication.
	if rec := serve("/x", "127.0.0.1:5000", "localhost", nil); rec.Code != 401 {
		t.Fatalf("/x: %d", rec.Code)
	}
}

func TestDiscovery(t *testing.T) {
	e := newEnv(t, nil)
	e.s.httpPort.Store(8097)
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.s.serveDiscovery(ctx, pc) }()

	// Garbage gets no answer.
	c, err := net.Dial("udp4", pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Write([]byte("HELLO"))
	_, _ = c.Write([]byte(DiscoveryMagic + strings.Repeat("x", 100)))
	_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if n, err := c.Read(make([]byte, 100)); err == nil {
		t.Fatalf("answered garbage (%d bytes)", n)
	}
	c.Close()

	found, err := discover(context.Background(), port, 500*time.Millisecond, []net.IP{net.IPv4(127, 0, 0, 1)})
	if err != nil || len(found) != 1 {
		t.Fatalf("discover: %+v %v", found, err)
	}
	d := found[0]
	if d.UID != e.s.UID() || d.Name != e.s.Name() || d.Port != 8097 || !d.TLS || d.Addr != "127.0.0.1" ||
		d.URL != "https://127.0.0.1:8097" || d.Version != "1.2.3" || d.Fingerprint != e.s.Fingerprint() {
		t.Fatalf("discovered: %+v", d)
	}
	// Cancellation returns early.
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	start := time.Now()
	if _, err := discover(cctx, port, 5*time.Second, []net.IP{net.IPv4(127, 0, 0, 1)}); !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := Discover(context.Background(), 0, time.Millisecond); err == nil {
		t.Fatal("port 0 accepted")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("responder did not stop")
	}
	if localSource(net.IPv4(8, 8, 8, 8)) || !localSource(net.IPv4(100, 100, 1, 1)) || !localSource(net.IPv4(192, 168, 0, 1)) {
		t.Fatal("localSource")
	}
}

func TestRunDiscovery(t *testing.T) {
	probe, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	s, err := New(Config{Listen: "127.0.0.1:0", DiscoveryPort: port, DataDir: t.TempDir(), Logger: quietLogger()}, "v")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	var found []Discovered
	for range 20 {
		found, _ = discover(context.Background(), port, 200*time.Millisecond, []net.IP{net.IPv4(127, 0, 0, 1)})
		if len(found) > 0 {
			break
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Port != int(s.httpPort.Load()) || found[0].UID != s.UID() {
		t.Fatalf("found %+v", found)
	}
}

func TestLoadConfigV2(t *testing.T) {
	t.Setenv("AETHER_AGENT_LISTEN", "")
	t.Setenv("AETHER_AGENT_DATA_DIR", "")
	p := filepath.Join(t.TempDir(), "agent.toml")
	writeFile(t, p, "name = \"Grenier\"\nbrowse_roots = [\"/srv\", \"/mnt\"]\ndiscovery_port = 0\n")
	cfg, err := LoadConfig(p)
	if err != nil || cfg.Name != "Grenier" || len(cfg.BrowseRoots) != 2 || cfg.DiscoveryPort != -1 {
		t.Fatalf("%+v %v", cfg, err)
	}
	writeFile(t, p, "discovery_port = 7999\n")
	if cfg, _ = LoadConfig(p); cfg.DiscoveryPort != 7999 {
		t.Fatalf("%+v", cfg)
	}
	if _, err := New(Config{DataDir: t.TempDir(), BrowseRoots: []string{"rel"}, Logger: quietLogger()}, "v"); err == nil {
		t.Fatal("relative browse root accepted")
	}
}

func TestHiddenEntriesNeverExposed(t *testing.T) {
	cases := map[string]bool{".ssh/id_rsa": true, "Films/.cache/x": true, "Films/Heat (1995)/heat.mkv": false, "": false}
	for p, want := range cases {
		if got := hiddenPath(p); got != want {
			t.Errorf("hiddenPath(%q) = %v", p, got)
		}
	}
}
