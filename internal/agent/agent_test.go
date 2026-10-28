package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

type env struct {
	t     *testing.T
	s     *Server
	ts    *httptest.Server
	share string // share root on disk
	data  string
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newEnv(t *testing.T, mut func(*Config)) *env {
	t.Helper()
	share := t.TempDir()
	data := t.TempDir()
	cfg := Config{
		Shares:  []Share{{Name: "films", Path: share}},
		DataDir: data,
		Logger:  quietLogger(),
	}
	if mut != nil {
		mut(&cfg)
	}
	s, err := New(cfg, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		ts.Close()
		s.Close()
	})
	resolved, _ := filepath.EvalSymlinks(share)
	return &env{t: t, s: s, ts: ts, share: resolved, data: data}
}

func (e *env) url(endpoint string, q url.Values) string {
	u := e.ts.URL + apiPrefix + endpoint
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// result is a fully read response (the body is closed by do).
type result struct {
	StatusCode int
	Header     http.Header
	Body       io.Reader
}

func (e *env) do(method, u, token string, hdr map[string]string) result {
	e.t.Helper()
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return result{StatusCode: resp.StatusCode, Header: resp.Header, Body: bytes.NewReader(b)}
}

// status performs a request and returns its status and JSON error code.
func (e *env) status(method, u, token string) (int, string) {
	e.t.Helper()
	resp := e.do(method, u, token, nil)
	var ae apiError
	_ = json.NewDecoder(resp.Body).Decode(&ae)
	return resp.StatusCode, ae.Error
}

func (e *env) get(endpoint string, q url.Values) result {
	return e.do(http.MethodGet, e.url(endpoint, q), e.s.Token(), nil)
}

func q(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func errCode(t *testing.T, resp result) string {
	t.Helper()
	var ae apiError
	if err := json.NewDecoder(resp.Body).Decode(&ae); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return ae.Error
}

func TestAuth(t *testing.T) {
	e := newEnv(t, nil)
	writeFile(t, filepath.Join(e.share, "a.mkv"), "hello")
	tok := e.s.Token()
	e.s.limiter.max = 100 // this test fails on purpose more than 5 times

	if st, code := e.status("GET", e.url("/info", nil), ""); st != 401 || code != "unauthorized" {
		t.Fatalf("no token: %d %s", st, code)
	}
	if st, _ := e.status("GET", e.url("/info", nil), "wrong"); st != 401 {
		t.Fatalf("wrong token: %d", st)
	}
	if st, _ := e.status("GET", e.url("/info", nil), tok); st != 200 {
		t.Fatalf("header: %d", st)
	}
	if r := e.do("GET", e.url("/info", nil), "", map[string]string{"Authorization": "bearer " + tok}); r.StatusCode != 200 {
		t.Fatalf("lowercase scheme: %d", r.StatusCode)
	}
	if r := e.do("GET", e.url("/info", q("t", tok)), "", nil); r.StatusCode != 401 {
		t.Fatalf("query on /info must be refused: %d", r.StatusCode)
	}
	if r := e.do("GET", e.url("/stat", q("share", "films", "path", "a.mkv", "t", tok)), "", nil); r.StatusCode != 401 {
		t.Fatalf("query on /stat must be refused: %d", r.StatusCode)
	}
	r := e.do("GET", e.url("/file", q("share", "films", "path", "a.mkv", "t", tok)), "", nil)
	if r.StatusCode != 200 || readAll(t, r.Body) != "hello" {
		t.Fatalf("query on /file: %d", r.StatusCode)
	}
	if r := e.do("GET", e.url("/file", q("share", "films", "path", "a.mkv", "t", "nope")), "", nil); r.StatusCode != 401 {
		t.Fatalf("bad query token: %d", r.StatusCode)
	}
	if r := e.do("GET", e.ts.URL+"/elsewhere", tok, nil); r.StatusCode != 404 {
		t.Fatalf("unknown route: %d", r.StatusCode)
	}
	if r := e.do("GET", e.ts.URL+"/elsewhere", "", nil); r.StatusCode != 401 {
		t.Fatalf("unknown route without auth: %d", r.StatusCode)
	}
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t, nil)
	now := time.Now()
	e.s.limiter.now = func() time.Time { return now }
	for i := range 5 {
		if r := e.do("GET", e.url("/info", nil), "bad", nil); r.StatusCode != 401 {
			t.Fatalf("attempt %d: %d", i, r.StatusCode)
		}
	}
	r := e.do("GET", e.url("/info", nil), e.s.Token(), nil)
	if r.StatusCode != 429 || r.Header.Get("Retry-After") == "" {
		t.Fatalf("expected 429 once blocked, got %d", r.StatusCode)
	}
	now = now.Add(61 * time.Second)
	if r := e.do("GET", e.url("/info", nil), e.s.Token(), nil); r.StatusCode != 200 {
		t.Fatalf("after window: %d", r.StatusCode)
	}
	// Successful requests do not consume the budget.
	for range 10 {
		if r := e.do("GET", e.url("/info", nil), e.s.Token(), nil); r.StatusCode != 200 {
			t.Fatal(r.StatusCode)
		}
	}
}

func TestLimiterCleanup(t *testing.T) {
	l := newFailLimiter(5, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }
	for i := range limiterMaxEntries {
		l.fail(fmt.Sprintf("10.0.%d.%d", i/256, i%256))
	}
	now = now.Add(2 * time.Minute)
	l.fail("192.0.2.1")
	if len(l.m) != 1 {
		t.Fatalf("expired entries not purged: %d", len(l.m))
	}
	now = now.Add(2 * time.Minute)
	for i := range limiterMaxEntries + 10 {
		l.fail(fmt.Sprintf("10.1.%d.%d", i/256, i%256))
	}
	if len(l.m) > limiterMaxEntries {
		t.Fatalf("limiter unbounded: %d", len(l.m))
	}
}

func TestPathValidation(t *testing.T) {
	e := newEnv(t, nil)
	writeFile(t, filepath.Join(e.share, "a/b.mkv"), "x")
	for _, p := range []string{"../etc", "a/../../x", "/etc/passwd", "a\x00b", `a\b`, ".."} {
		for _, ep := range []string{"/stat", "/readdir", "/walk", "/file"} {
			r := e.get(ep, q("share", "films", "path", p))
			if r.StatusCode != 400 || errCode(t, r) != "invalid_path" {
				t.Errorf("%s %q: %d", ep, p, r.StatusCode)
			}
		}
	}
	if r := e.get("/stat", q("path", "a")); r.StatusCode != 400 {
		t.Errorf("missing share: %d", r.StatusCode)
	}
	if r := e.get("/stat", q("share", "nope", "path", "a")); r.StatusCode != 404 {
		t.Errorf("unknown share: %d", r.StatusCode)
	}
	// Harmless forms are normalised.
	for _, p := range []string{"a/", "./a", "a//b.mkv"} {
		if r := e.get("/stat", q("share", "films", "path", p)); r.StatusCode != 200 {
			t.Errorf("%q: %d", p, r.StatusCode)
		}
	}
}

func TestSymlinkEscape(t *testing.T) {
	e := newEnv(t, nil)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.mkv"), "secret")
	writeFile(t, filepath.Join(e.share, "in/real.mkv"), "inside")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Symlink(filepath.Join(outside, "secret.mkv"), filepath.Join(e.share, "leak.mkv")))
	must(os.Symlink(outside, filepath.Join(e.share, "leakdir")))
	must(os.Symlink("../in/real.mkv", filepath.Join(e.share, "in", "..", "alias.mkv")))
	must(os.Symlink("real.mkv", filepath.Join(e.share, "in", "rel.mkv")))

	for _, p := range []string{"leak.mkv", "leakdir", "leakdir/secret.mkv"} {
		for _, ep := range []string{"/stat", "/file", "/readdir", "/walk"} {
			r := e.get(ep, q("share", "films", "path", p))
			if r.StatusCode != 404 {
				t.Errorf("%s %q: %d (%s)", ep, p, r.StatusCode, readAll(t, r.Body))
			}
		}
		r := e.do("POST", e.url("/probe", q("share", "films", "path", p)), e.s.Token(), nil)
		if r.StatusCode != 404 && (r.StatusCode != 500 || e.s.ffprobePath != "") {
			t.Errorf("probe %q: %d", p, r.StatusCode)
		}
	}
	// Internal links work.
	r := e.get("/file", q("share", "films", "path", "in/rel.mkv"))
	if r.StatusCode != 200 || readAll(t, r.Body) != "inside" {
		t.Fatalf("internal symlink: %d", r.StatusCode)
	}
	// Escaping links never show up in listings.
	body := readAll(t, e.get("/walk", q("share", "films")).Body)
	if strings.Contains(body, "leak") || strings.Contains(body, "secret") {
		t.Fatalf("walk leaked: %s", body)
	}
	if !strings.Contains(body, `"path":"in/rel.mkv"`) {
		t.Fatalf("internal link missing: %s", body)
	}
	var list []vfs.Entry
	_ = json.NewDecoder(e.get("/readdir", q("share", "films")).Body).Decode(&list)
	for _, en := range list {
		if strings.HasPrefix(en.Path, "leak") {
			t.Fatalf("readdir leaked %s", en.Path)
		}
	}
	// localPath (used for ffprobe) refuses escapes too.
	sh := e.s.shares["films"]
	if _, err := sh.localPath("leak.mkv"); err == nil {
		t.Fatal("localPath accepted escaping link")
	}
	if p, err := sh.localPath("in/rel.mkv"); err != nil || p != filepath.Join(e.share, "in", "real.mkv") {
		t.Fatalf("localPath internal: %q %v", p, err)
	}
}

type walkLine struct {
	vfs.Entry
	Done  bool   `json:"done"`
	Error string `json:"error"`
}

func readWalk(t *testing.T, r io.Reader) ([]vfs.Entry, walkLine) {
	t.Helper()
	var entries []vfs.Entry
	var last walkLine
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var l walkLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("bad line %q: %v", sc.Text(), err)
		}
		if l.Done || l.Error != "" {
			last = l
			if sc.Scan() {
				t.Fatalf("data after final line: %q", sc.Text())
			}
			break
		}
		entries = append(entries, l.Entry)
	}
	return entries, last
}

func TestWalk(t *testing.T) {
	e := newEnv(t, nil)
	writeFile(t, filepath.Join(e.share, "Films/Heat (1995)/heat.mkv"), "12345")
	writeFile(t, filepath.Join(e.share, "Films/Heat (1995)/heat.nfo"), "n")
	writeFile(t, filepath.Join(e.share, "Films/Alien/alien.mp4"), "a")
	writeFile(t, filepath.Join(e.share, "top.txt"), "t")
	if err := os.Mkdir(filepath.Join(e.share, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := e.get("/walk", q("share", "films"))
	if r.StatusCode != 200 || r.Header.Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("%d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	entries, last := readWalk(t, r.Body)
	if !last.Done {
		t.Fatalf("no done line: %+v", last)
	}
	var got []string
	for _, en := range entries {
		got = append(got, en.Path)
	}
	want := []string{"Films", "Films/Alien", "Films/Alien/alien.mp4", "Films/Heat (1995)",
		"Films/Heat (1995)/heat.mkv", "Films/Heat (1995)/heat.nfo", "empty", "top.txt"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("order:\n got %v\nwant %v", got, want)
	}
	for _, en := range entries {
		if en.Path == "Films/Heat (1995)/heat.mkv" && (en.Size != 5 || en.IsDir || en.ModTime.IsZero()) {
			t.Fatalf("bad entry %+v", en)
		}
		if en.Path == "Films" && !en.IsDir {
			t.Fatalf("dir flag missing")
		}
	}

	// Sub-path walk: paths stay relative to the share.
	entries, last = readWalk(t, e.get("/walk", q("share", "films", "path", "Films/Heat (1995)")).Body)
	if !last.Done || len(entries) != 2 || entries[0].Path != "Films/Heat (1995)/heat.mkv" {
		t.Fatalf("sub walk: %+v %+v", entries, last)
	}

	if r := e.get("/walk", q("share", "films", "path", "missing")); r.StatusCode != 404 {
		t.Fatalf("missing: %d", r.StatusCode)
	}
	if r := e.get("/walk", q("share", "films", "path", "top.txt")); r.StatusCode != 400 {
		t.Fatalf("file: %d", r.StatusCode)
	}
}

func TestWalkSkipsUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	e := newEnv(t, nil)
	writeFile(t, filepath.Join(e.share, "a/locked/x.mkv"), "x")
	writeFile(t, filepath.Join(e.share, "b/y.mkv"), "y")
	locked := filepath.Join(e.share, "a/locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	entries, last := readWalk(t, e.get("/walk", q("share", "films")).Body)
	if !last.Done {
		t.Fatalf("walk failed: %+v", last)
	}
	paths := map[string]bool{}
	for _, en := range entries {
		paths[en.Path] = true
	}
	if !paths["a/locked"] || paths["a/locked/x.mkv"] || !paths["b/y.mkv"] {
		t.Fatalf("unexpected entries: %v", paths)
	}
}

func TestWalkLarge(t *testing.T) {
	e := newEnv(t, nil)
	e.s.walkFlushN = 500
	const dirs, per = 50, 99 // 50 dirs + 4950 files = 5000 entries
	for d := range dirs {
		dir := filepath.Join(e.share, fmt.Sprintf("d%02d", d))
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for f := range per {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d.mkv", f)), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	r := e.get("/walk", q("share", "films"))
	br := bufio.NewReader(r.Body)
	first, err := br.ReadString('\n')
	if err != nil || !strings.Contains(first, `"path":"d00"`) {
		t.Fatalf("first line %q %v", first, err)
	}
	entries, last := readWalk(t, br)
	if !last.Done || len(entries)+1 != dirs*(per+1) {
		t.Fatalf("got %d entries, last %+v", len(entries)+1, last)
	}
	seen := map[string]bool{"d00": true}
	for _, en := range entries {
		if parent := filepath.Dir(en.Path); parent != "." && !seen[parent] {
			t.Fatalf("%s before its parent", en.Path)
		}
		seen[en.Path] = true
	}
}

func TestWalkClientCancel(t *testing.T) {
	e := newEnv(t, nil)
	e.s.walkFlushN = 1
	for i := range 2000 {
		writeFile(t, filepath.Join(e.share, fmt.Sprintf("d/%04d", i)), "")
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", e.url("/walk", q("share", "films")), nil)
	req.Header.Set("Authorization", "Bearer "+e.s.Token())
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(resp.Body).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	cancel()
	resp.Body.Close()
	// ts.Close in cleanup would hang if the handler did not stop.
}

func TestStatReadDir(t *testing.T) {
	e := newEnv(t, nil)
	writeFile(t, filepath.Join(e.share, "dir/b.mkv"), "bb")
	writeFile(t, filepath.Join(e.share, "dir/a.srt"), "a")
	if err := os.Mkdir(filepath.Join(e.share, "dir/sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	var en vfs.Entry
	r := e.get("/stat", q("share", "films", "path", "dir/b.mkv"))
	if err := json.NewDecoder(r.Body).Decode(&en); err != nil || en.Path != "dir/b.mkv" || en.Size != 2 || en.IsDir {
		t.Fatalf("stat: %+v %v", en, err)
	}
	r = e.get("/stat", q("share", "films"))
	if err := json.NewDecoder(r.Body).Decode(&en); err != nil || en.Path != "" || !en.IsDir {
		t.Fatalf("stat root: %+v %v", en, err)
	}
	r = e.get("/stat", q("share", "films", "path", "nope.mkv"))
	if r.StatusCode != 404 || errCode(t, r) != "not_found" {
		t.Fatalf("stat missing: %d", r.StatusCode)
	}

	var list []vfs.Entry
	r = e.get("/readdir", q("share", "films", "path", "dir"))
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].Path != "dir/a.srt" || list[1].Path != "dir/b.mkv" || list[2].Path != "dir/sub" || !list[2].IsDir {
		t.Fatalf("readdir: %+v", list)
	}
	r = e.get("/readdir", q("share", "films", "path", "dir/sub"))
	if b := strings.TrimSpace(readAll(t, r.Body)); b != "[]" {
		t.Fatalf("empty dir must be [] not %q", b)
	}
	if r := e.get("/readdir", q("share", "films", "path", "missing")); r.StatusCode != 404 {
		t.Fatalf("readdir missing: %d", r.StatusCode)
	}
	if r := e.get("/readdir", q("share", "films", "path", "dir/b.mkv")); r.StatusCode != 400 {
		t.Fatalf("readdir file: %d", r.StatusCode)
	}
}

func TestFile(t *testing.T) {
	e := newEnv(t, nil)
	writeFile(t, filepath.Join(e.share, "m/movie.mkv"), "0123456789")
	writeFile(t, filepath.Join(e.share, "m/blob.xyz"), "z")
	u := e.url("/file", q("share", "films", "path", "m/movie.mkv"))

	r := e.do("GET", u, e.s.Token(), nil)
	if r.StatusCode != 200 || readAll(t, r.Body) != "0123456789" {
		t.Fatalf("get: %d", r.StatusCode)
	}
	if r.Header.Get("Content-Type") != "video/x-matroska" || r.Header.Get("Accept-Ranges") != "bytes" ||
		r.Header.Get("Last-Modified") == "" || r.Header.Get("Content-Length") != "10" {
		t.Fatalf("headers: %v", r.Header)
	}
	r = e.do("GET", u, e.s.Token(), map[string]string{"Range": "bytes=2-5"})
	if r.StatusCode != 206 || readAll(t, r.Body) != "2345" || r.Header.Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("range: %d %v", r.StatusCode, r.Header)
	}
	r = e.do("HEAD", u, e.s.Token(), nil)
	if r.StatusCode != 200 || r.Header.Get("Content-Length") != "10" || readAll(t, r.Body) != "" {
		t.Fatalf("head: %d %v", r.StatusCode, r.Header)
	}
	lm := r.Header.Get("Last-Modified")
	r = e.do("GET", u, e.s.Token(), map[string]string{"If-Modified-Since": lm})
	if r.StatusCode != 304 {
		t.Fatalf("conditional: %d", r.StatusCode)
	}
	r = e.get("/file", q("share", "films", "path", "m/blob.xyz"))
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("default type: %s", r.Header.Get("Content-Type"))
	}
	for _, p := range []string{"", "m"} {
		if r := e.get("/file", q("share", "films", "path", p)); r.StatusCode != 400 {
			t.Fatalf("dir %q: %d", p, r.StatusCode)
		}
	}
	if r := e.get("/file", q("share", "films", "path", "m/none.mkv")); r.StatusCode != 404 {
		t.Fatalf("missing: %d", r.StatusCode)
	}
}

func TestInfo(t *testing.T) {
	for _, expose := range []bool{false, true} {
		e := newEnv(t, func(c *Config) { c.ExposePaths = expose })
		var info struct {
			Version  string
			Hostname string
			FFprobe  *bool
			Shares   []map[string]string
		}
		if err := json.NewDecoder(e.get("/info", nil).Body).Decode(&info); err != nil {
			t.Fatal(err)
		}
		if info.Version != "1.2.3" || info.FFprobe == nil || len(info.Shares) != 1 || info.Shares[0]["name"] != "films" {
			t.Fatalf("info: %+v", info)
		}
		if _, has := info.Shares[0]["path"]; has != expose {
			t.Fatalf("expose=%v but path present=%v", expose, has)
		}
	}
}

func TestProbe(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	e := newEnv(t, nil)
	if e.s.ffprobePath == "" {
		t.Skip("ffprobe not installed")
	}
	media := filepath.Join(e.share, "tiny clip.mkv")
	out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=1:size=64x64:rate=5",
		"-c:v", "mpeg4", "-y", media).CombinedOutput()
	if err != nil {
		t.Skipf("ffmpeg cannot generate media: %v %s", err, out)
	}
	writeFile(t, filepath.Join(e.share, "garbage.mkv"), strings.Repeat("not a video ", 100))

	r := e.do("POST", e.url("/probe", q("share", "films", "path", "tiny clip.mkv")), e.s.Token(), nil)
	if r.StatusCode != 200 {
		t.Fatalf("probe: %d %s", r.StatusCode, readAll(t, r.Body))
	}
	var pr struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
		}
		Format struct {
			FormatName string `json:"format_name"`
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&pr); err != nil || len(pr.Streams) != 1 || pr.Streams[0].CodecType != "video" ||
		!strings.Contains(pr.Format.FormatName, "matroska") {
		t.Fatalf("probe output: %+v %v", pr, err)
	}

	r = e.do("POST", e.url("/probe", q("share", "films", "path", "garbage.mkv")), e.s.Token(), nil)
	if r.StatusCode != 422 {
		t.Fatalf("garbage: %d", r.StatusCode)
	}
	var ae apiError
	if err := json.NewDecoder(r.Body).Decode(&ae); err != nil || ae.Error != "probe_failed" || ae.Message == "" {
		t.Fatalf("garbage body: %+v %v", ae, err)
	}
	if r := e.do("POST", e.url("/probe", q("share", "films", "path", "none.mkv")), e.s.Token(), nil); r.StatusCode != 404 {
		t.Fatalf("missing: %d", r.StatusCode)
	}
	if r := e.do("POST", e.url("/probe", q("share", "films")), e.s.Token(), nil); r.StatusCode != 400 {
		t.Fatalf("root: %d", r.StatusCode)
	}
	if r := e.do("GET", e.url("/probe", q("share", "films", "path", "tiny clip.mkv")), e.s.Token(), nil); r.StatusCode == 200 {
		t.Fatal("GET probe must not succeed")
	}
}

func TestProbeUnavailable(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.FFprobe = "definitely-not-ffprobe-xyz" })
	writeFile(t, filepath.Join(e.share, "a.mkv"), "x")
	r := e.do("POST", e.url("/probe", q("share", "films", "path", "a.mkv")), e.s.Token(), nil)
	if r.StatusCode != 500 || errCode(t, r) != "ffprobe_unavailable" {
		t.Fatalf("%d", r.StatusCode)
	}
}

type sseEvent struct{ name, data string }

func readSSE(body io.Reader, out chan<- sseEvent) {
	sc := bufio.NewScanner(body)
	var ev sseEvent
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if ev.name != "" || ev.data != "" {
				out <- ev
			}
			ev = sseEvent{}
		case strings.HasPrefix(line, ":"):
			out <- sseEvent{name: "comment", data: strings.TrimSpace(line[1:])}
		case strings.HasPrefix(line, "event: "):
			ev.name = line[7:]
		case strings.HasPrefix(line, "data: "):
			ev.data += line[6:]
		}
	}
	close(out)
}

func TestEvents(t *testing.T) {
	e := newEnv(t, nil)
	e.s.debounce = 150 * time.Millisecond
	e.s.pingEvery = 100 * time.Millisecond
	writeFile(t, filepath.Join(e.share, "Films/old.mkv"), "x")

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

	subscribe := func() chan sseEvent {
		req, _ := http.NewRequestWithContext(ctx, "GET", e.url("/events", nil), nil)
		req.Header.Set("Authorization", "Bearer "+e.s.Token())
		resp, err := e.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		if resp.Header.Get("Content-Type") != "text/event-stream" {
			t.Fatalf("content type %s", resp.Header.Get("Content-Type"))
		}
		ch := make(chan sseEvent, 100)
		go readSSE(resp.Body, ch)
		return ch
	}
	sub1, sub2 := subscribe(), subscribe()
	time.Sleep(200 * time.Millisecond) // let the watcher register

	writeFile(t, filepath.Join(e.share, "Films/new.mkv"), "y")
	if err := os.MkdirAll(filepath.Join(e.share, "Series/Show/S01"), 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	writeFile(t, filepath.Join(e.share, "Series/Show/S01/e01.mkv"), "z")

	waitChange := func(ch chan sseEvent, want ...string) {
		t.Helper()
		got := map[string]bool{}
		pings := 0
		deadline := time.After(5 * time.Second)
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					t.Fatal("stream closed")
				}
				if ev.name == "comment" && ev.data == "ping" {
					pings++
					continue
				}
				if ev.name != "change" {
					continue
				}
				var ce changeEvent
				if err := json.Unmarshal([]byte(ev.data), &ce); err != nil || ce.Share != "films" {
					t.Fatalf("bad event %q %v", ev.data, err)
				}
				for _, p := range ce.Paths {
					got[p] = true
				}
				all := true
				for _, w := range want {
					covered := false
					for p := range got {
						if p == w || p == "" || strings.HasPrefix(w, p+"/") {
							covered = true
						}
					}
					all = all && covered
				}
				if all && pings > 0 {
					return
				}
			case <-deadline:
				t.Fatalf("timeout; got %v pings %d", got, pings)
			}
		}
	}
	waitChange(sub1, "Films/new.mkv", "Series/Show/S01/e01.mkv")
	waitChange(sub2, "Films/new.mkv", "Series/Show/S01/e01.mkv")
}

func TestHubDropsSlowSubscriber(t *testing.T) {
	h := newHub()
	slow, _ := h.subscribe()
	fast, _ := h.subscribe()
	for range subscriberQueue + 1 {
		h.publish([]byte("x"))
		<-fast
	}
	n := 0
	for range slow {
		n++
	}
	if n != subscriberQueue {
		t.Fatalf("slow subscriber got %d before being dropped", n)
	}
	h.publish([]byte("y"))
	if m := <-fast; string(m) != "y" {
		t.Fatal("fast subscriber lost")
	}
	h.close()
	if _, ok := <-fast; ok {
		t.Fatal("close must end subscribers")
	}
	if _, ok := h.subscribe(); ok {
		t.Fatal("subscribe after close")
	}
}

func TestReducePaths(t *testing.T) {
	got := reducePaths([]string{"a/b", "a", "a/b/c", "x/y", "x/y", "x/z"}, 500)
	if strings.Join(got, ",") != "a,x/y,x/z" {
		t.Fatalf("%v", got)
	}
	var many []string
	for i := range 600 {
		many = append(many, fmt.Sprintf("d%d/f%d", i%3, i))
	}
	got = reducePaths(many, 500)
	if strings.Join(got, ",") != "d0,d1,d2" {
		t.Fatalf("%v", got)
	}
	many = many[:0]
	for i := range 600 {
		many = append(many, fmt.Sprintf("f%d", i))
	}
	if got = reducePaths(many, 500); len(got) != 1 || got[0] != "" {
		t.Fatalf("%v", got)
	}
}

func TestTokenPersistence(t *testing.T) {
	share, data := t.TempDir(), filepath.Join(t.TempDir(), "nested")
	cfg := Config{Shares: []Share{{Name: "a", Path: share}}, DataDir: data, Logger: quietLogger()}
	s1, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()
	tok := s1.Token()
	if len(tok) != 43 || strings.ContainsAny(tok, "+/=") {
		t.Fatalf("token %q is not 32 bytes base64url", tok)
	}
	fi, err := os.Stat(filepath.Join(data, tokenFileName))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file: %v %v", fi.Mode(), err)
	}
	s2, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
	if s2.Token() != tok {
		t.Fatal("token not persisted")
	}
	// Loose permissions are tightened.
	_ = os.Chmod(filepath.Join(data, tokenFileName), 0o644)
	s3, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	s3.Close()
	if fi, _ := os.Stat(filepath.Join(data, tokenFileName)); fi.Mode().Perm() != 0o600 {
		t.Fatalf("perms not fixed: %v", fi.Mode())
	}
	// A corrupt token file is an error, not silently replaced.
	_ = os.WriteFile(filepath.Join(data, tokenFileName), []byte("short"), 0o600)
	if _, err := New(cfg, "v"); err == nil {
		t.Fatal("short token accepted")
	}
}

func TestNewValidation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	writeFile(t, file, "")
	cases := map[string][]Share{
		"none":      nil,
		"upper":     {{Name: "Films", Path: dir}},
		"space":     {{Name: "my films", Path: dir}},
		"long":      {{Name: strings.Repeat("a", 41), Path: dir}},
		"empty":     {{Name: "", Path: dir}},
		"duplicate": {{Name: "a", Path: dir}, {Name: "a", Path: dir}},
		"missing":   {{Name: "a", Path: filepath.Join(dir, "nope")}},
		"notdir":    {{Name: "a", Path: file}},
		"nopath":    {{Name: "a"}},
	}
	for name, shares := range cases {
		if _, err := New(Config{Shares: shares, DataDir: t.TempDir(), Logger: quietLogger()}, "v"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := New(Config{Shares: []Share{{Name: "ok_1-x", Path: dir}}, DataDir: t.TempDir(), TLSCert: "c", Logger: quietLogger()}, "v"); err == nil {
		t.Error("cert without key accepted")
	}
	s, err := New(Config{Shares: []Share{{Name: "ok_1-x", Path: dir}}, DataDir: t.TempDir(), Logger: quietLogger()}, "v")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.toml")
	writeFile(t, p, `
listen = "127.0.0.1:9000"
tls_cert = "/c.pem"
tls_key  = "/k.pem"
data_dir = "/var/lib/aether-agent"
expose_paths = true
[[share]]
name = "films"
path = "/srv/media/films"
[[share]]
name = "series"
path = "/srv/media/series"
`)
	t.Setenv("AETHER_AGENT_LISTEN", "")
	t.Setenv("AETHER_AGENT_DATA_DIR", "")
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9000" || cfg.TLSCert != "/c.pem" || cfg.TLSKey != "/k.pem" || !cfg.ExposePaths ||
		cfg.DataDir != "/var/lib/aether-agent" || cfg.FFprobe != "ffprobe" || len(cfg.Shares) != 2 ||
		cfg.Shares[1] != (Share{Name: "series", Path: "/srv/media/series"}) {
		t.Fatalf("%+v", cfg)
	}
	t.Setenv("AETHER_AGENT_LISTEN", ":1234")
	t.Setenv("AETHER_AGENT_DATA_DIR", "/tmp/x")
	cfg, err = LoadConfig(p)
	if err != nil || cfg.Listen != ":1234" || cfg.DataDir != "/tmp/x" {
		t.Fatalf("env override: %+v %v", cfg, err)
	}
	t.Setenv("AETHER_AGENT_LISTEN", "")
	writeFile(t, p, "[[share]]\nname = \"a\"\npath = \"/x\"\n")
	if cfg, err = LoadConfig(p); err != nil || cfg.Listen != DefaultListen {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	writeFile(t, p, "listne = \"x\"\n")
	if _, err := LoadConfig(p); err == nil || !strings.Contains(err.Error(), "listne") {
		t.Fatalf("unknown key: %v", err)
	}
	writeFile(t, p, "listen = \n")
	if _, err := LoadConfig(p); err == nil {
		t.Fatal("syntax error accepted")
	}
	if _, err := LoadConfig(filepath.Join(dir, "missing.toml")); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestRun(t *testing.T) {
	cfg := Config{Listen: "127.0.0.1:0", Shares: []Share{{Name: "a", Path: t.TempDir()}}, DataDir: t.TempDir(), Logger: quietLogger()}
	s, err := New(cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not stop")
	}

	busy := Config{Listen: "256.0.0.1:1", Shares: cfg.Shares, DataDir: cfg.DataDir, Logger: quietLogger()}
	s2, err := New(busy, "v")
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.Run(context.Background()); err == nil {
		t.Fatal("bad listen address accepted")
	}
}
