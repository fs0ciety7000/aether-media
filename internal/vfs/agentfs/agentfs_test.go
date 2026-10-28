package agentfs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

const token = "tok-123_abc"

var mtime = time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)

// fakeAgent implements the agent protocol minimally over an in-memory share.
type fakeAgent struct {
	mu        sync.Mutex
	files     map[string][]byte // share-relative path → content
	dirs      map[string]bool
	status    int // forced status for every request when != 0
	gets      atomic.Int64
	ignoreRng bool
}

func newFake() *fakeAgent {
	return &fakeAgent{files: map[string][]byte{}, dirs: map[string]bool{}}
}

func (a *fakeAgent) add(p string, b []byte) {
	a.files[p] = b
	for d := p; strings.Contains(d, "/"); {
		d = d[:strings.LastIndex(d, "/")]
		a.dirs[d] = true
	}
}

func (a *fakeAgent) entries(prefix string) []vfs.Entry {
	var out []vfs.Entry
	under := func(p string) bool { return prefix == "" || strings.HasPrefix(p, prefix+"/") }
	for d := range a.dirs {
		if under(d) {
			out = append(out, vfs.Entry{Path: d, ModTime: mtime, IsDir: true})
		}
	}
	for p, b := range a.files {
		if under(p) {
			out = append(out, vfs.Entry{Path: p, Size: int64(len(b)), ModTime: mtime})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path }) // parents first
	return out
}

func writeErr(w http.ResponseWriter, code int, c string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q,"message":"x"}`, c)
}

func (a *fakeAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	forced := a.status
	a.mu.Unlock()
	if forced != 0 {
		writeErr(w, forced, "forced")
		return
	}
	q := r.URL.Query()
	auth := r.Header.Get("Authorization") == "Bearer "+token
	if r.URL.Path == "/agent/v1/file" && q.Get("t") == token {
		auth = true
	}
	if !auth {
		writeErr(w, 401, "unauthorized")
		return
	}
	if q.Get("share") != "" && q.Get("share") != "films" {
		writeErr(w, 404, "not_found")
		return
	}
	p := q.Get("path")
	a.mu.Lock()
	defer a.mu.Unlock()
	switch r.URL.Path {
	case "/agent/v1/info":
		json.NewEncoder(w).Encode(AgentInfo{Version: "1.0", Hostname: "nas", Shares: []Share{{Name: "films"}}})
	case "/agent/v1/walk":
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		for _, e := range a.entries(p) {
			enc.Encode(e)
		}
		io.WriteString(w, `{"done":true}`+"\n")
	case "/agent/v1/stat":
		if b, ok := a.files[p]; ok {
			json.NewEncoder(w).Encode(vfs.Entry{Path: p, Size: int64(len(b)), ModTime: mtime})
			return
		}
		if a.dirs[p] || p == "" {
			json.NewEncoder(w).Encode(vfs.Entry{Path: p, ModTime: mtime, IsDir: true})
			return
		}
		writeErr(w, 404, "not_found")
	case "/agent/v1/readdir":
		var out []vfs.Entry
		for _, e := range a.entries(p) {
			rest := strings.TrimPrefix(e.Path, p+"/")
			if p == "" {
				rest = e.Path
			}
			if !strings.Contains(rest, "/") {
				out = append(out, e)
			}
		}
		json.NewEncoder(w).Encode(out)
	case "/agent/v1/file":
		b, ok := a.files[p]
		if !ok {
			writeErr(w, 404, "not_found")
			return
		}
		if r.Method == http.MethodGet {
			a.gets.Add(1)
		}
		if a.ignoreRng {
			r.Header.Del("Range")
		}
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, "", mtime, bytes.NewReader(b))
	default:
		writeErr(w, 404, "not_found")
	}
}

func newTestFS(t *testing.T, a *fakeAgent, root string) (*httptest.Server, *FS) {
	t.Helper()
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	f, err := newFS(Config{BaseURL: srv.URL + "/", Token: token, Share: "films", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return srv, f
}

func TestWalkStreaming(t *testing.T) {
	a := newFake()
	const n = 20000
	for i := range n {
		a.add(fmt.Sprintf("lib/d%03d/movie-%05d-%s.mkv", i%100, i, strings.Repeat("x", 40)), []byte("x"))
	}
	a.add("other/z.mkv", []byte("z"))
	_, f := newTestFS(t, a, "lib")
	ctx := context.Background()
	files, dirs := 0, 0
	err := f.Walk(ctx, "", func(e vfs.Entry) error {
		if strings.HasPrefix(e.Path, "lib") || strings.HasPrefix(e.Path, "other") {
			t.Fatalf("path not relativised: %q", e.Path)
		}
		if e.IsDir {
			dirs++
		} else {
			files++
		}
		return nil
	})
	if err != nil || files != n || dirs != 100 {
		t.Fatalf("files=%d dirs=%d err=%v", files, dirs, err)
	}
	// SkipDir on directories.
	files = 0
	err = f.Walk(ctx, "", func(e vfs.Entry) error {
		if e.IsDir && e.Path != "d000" {
			return fs.SkipDir
		}
		if !e.IsDir {
			files++
		}
		return nil
	})
	if err != nil || files != n/100 {
		t.Fatalf("skipdir files=%d err=%v", files, err)
	}
	// Sub-dir walk and SkipAll.
	cnt := 0
	err = f.Walk(ctx, "d001", func(e vfs.Entry) error {
		if !strings.HasPrefix(e.Path, "d001/") {
			t.Fatalf("bad path %q", e.Path)
		}
		cnt++
		if cnt == 5 {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil || cnt != 5 {
		t.Fatalf("cnt=%d err=%v", cnt, err)
	}
	// Callback error propagates.
	boom := errors.New("boom")
	if err := f.Walk(ctx, "", func(vfs.Entry) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestWalkTerminators(t *testing.T) {
	for _, tc := range []struct {
		body string
		want error
	}{
		{`{"path":"a","size":1,"mtime":"2026-10-28T10:00:00Z"}` + "\n" + `{"error":"io","message":"disk"}` + "\n", vfs.ErrOffline},
		{`{"path":"a","size":1,"mtime":"2026-10-28T10:00:00Z"}` + "\n", vfs.ErrOffline},
		{`{"error":"not_found"}` + "\n", vfs.ErrNotExist},
		{`{"path":"a","size":1,"mtime":"2026-10-28T10:00:00Z"}` + "\n\n" + `{"done":true}` + "\n", nil},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, tc.body) }))
		f, _ := newFS(Config{BaseURL: srv.URL, Token: token, Share: "films"})
		n := 0
		err := f.Walk(context.Background(), "", func(vfs.Entry) error { n++; return nil })
		srv.Close()
		if tc.want == nil && (err != nil || n != 1) || tc.want != nil && !errors.Is(err, tc.want) {
			t.Errorf("body %q: n=%d err=%v want %v", tc.body, n, err, tc.want)
		}
	}
}

func TestStatReadDir(t *testing.T) {
	a := newFake()
	a.add("lib/Heat/heat.mkv", []byte("12345"))
	a.add("lib/Heat/heat.nfo", []byte("n"))
	a.add("lib/top.mkv", []byte("t"))
	_, f := newTestFS(t, a, "lib")
	ctx := context.Background()
	e, err := f.Stat(ctx, "Heat/heat.mkv")
	if err != nil || e.Path != "Heat/heat.mkv" || e.Size != 5 || !e.ModTime.Equal(mtime) {
		t.Fatalf("stat = %+v %v", e, err)
	}
	if _, err := f.Stat(ctx, "nope"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.Stat(ctx, "../etc"); !errors.Is(err, vfs.ErrPath) {
		t.Fatalf("err = %v", err)
	}
	es, err := f.ReadDir(ctx, "")
	if err != nil || len(es) != 2 || es[0].Path != "Heat" || es[1].Path != "top.mkv" {
		t.Fatalf("readdir = %+v %v", es, err)
	}
	if err := f.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRangeReadsAndSeeks(t *testing.T) {
	for _, ignore := range []bool{false, true} {
		t.Run(fmt.Sprintf("ignoreRange=%v", ignore), func(t *testing.T) {
			a := newFake()
			a.ignoreRng = ignore
			data := make([]byte, 3<<20+17)
			for i := range data {
				data[i] = byte(rand.IntN(256))
			}
			a.add("big.bin", data)
			_, f := newTestFS(t, a, "")
			fl, err := f.Open(context.Background(), "big.bin")
			if err != nil {
				t.Fatal(err)
			}
			defer fl.Close()
			if fl.Size() != int64(len(data)) || !fl.ModTime().Equal(mtime) {
				t.Fatalf("size=%d mtime=%v", fl.Size(), fl.ModTime())
			}
			// Sequential read is one GET.
			got, err := io.ReadAll(fl)
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("readall: %v (len %d)", err, len(got))
			}
			if g := a.gets.Load(); g != 1 {
				t.Fatalf("sequential read used %d GETs", g)
			}
			// At EOF.
			if n, err := fl.Read(make([]byte, 10)); n != 0 || !errors.Is(err, io.EOF) {
				t.Fatalf("read at eof: %d %v", n, err)
			}
			// Random access.
			for range 200 {
				var off int64
				switch rand.IntN(3) {
				case 0:
					off, err = fl.Seek(rand.Int64N(int64(len(data))), io.SeekStart)
				case 1:
					off, err = fl.Seek(-rand.Int64N(int64(len(data))), io.SeekEnd)
				default:
					off, err = fl.Seek(rand.Int64N(100000), io.SeekCurrent)
				}
				if err != nil {
					t.Fatal(err)
				}
				buf := make([]byte, rand.IntN(200000))
				n, err := io.ReadFull(fl, buf)
				start := min(off, int64(len(data)))
				end := min(off+int64(len(buf)), int64(len(data)))
				end = max(end, start)
				if int64(n) != end-start || !bytes.Equal(buf[:n], data[start:end]) {
					t.Fatalf("mismatch off=%d n=%d err=%v", off, n, err)
				}
			}
			// Small forward seeks reuse the body.
			before := a.gets.Load()
			fl.Seek(0, io.SeekStart)
			b := make([]byte, 100)
			for i := range 10 {
				if _, err := io.ReadFull(fl, b); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(b, data[i*1100:i*1100+100]) {
					t.Fatal("mismatch after small seek")
				}
				fl.Seek(1000, io.SeekCurrent)
			}
			if g := a.gets.Load() - before; g != 1 {
				t.Fatalf("small forward seeks used %d GETs", g)
			}
			// Seek past EOF → EOF.
			fl.Seek(int64(len(data))+5, io.SeekStart)
			if _, err := fl.Read(b); !errors.Is(err, io.EOF) {
				t.Fatalf("read past eof: %v", err)
			}
			if _, err := fl.Seek(-1, io.SeekStart); err == nil {
				t.Fatal("negative seek accepted")
			}
		})
	}
}

func TestServeContentOverFile(t *testing.T) {
	a := newFake()
	data := bytes.Repeat([]byte("0123456789"), 100000)
	a.add("m.mkv", data)
	_, f := newTestFS(t, a, "")
	fl, err := f.Open(context.Background(), "m.mkv")
	if err != nil {
		t.Fatal(err)
	}
	defer fl.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Range", "bytes=500000-500009")
	http.ServeContent(rec, req, "m.mkv", fl.ModTime(), fl)
	if rec.Code != 206 || rec.Body.String() != "0123456789" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestEmptyFile(t *testing.T) {
	a := newFake()
	a.add("empty", nil)
	_, f := newTestFS(t, a, "")
	fl, err := f.Open(context.Background(), "empty")
	if err != nil {
		t.Fatal(err)
	}
	if b, err := io.ReadAll(fl); err != nil || len(b) != 0 {
		t.Fatalf("%q %v", b, err)
	}
}

func TestTokenAndInputURL(t *testing.T) {
	a := newFake()
	a.add("lib/Heat (1995)/heat & co.mkv", []byte("data"))
	srv, f := newTestFS(t, a, "lib")
	u := f.InputURL("Heat (1995)/heat & co.mkv")
	pu, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := pu.Query()
	if pu.Path != "/agent/v1/file" || q.Get("share") != "films" || q.Get("path") != "lib/Heat (1995)/heat & co.mkv" || q.Get("t") != token {
		t.Fatalf("InputURL = %s", u)
	}
	// The URL works without any header (as FFmpeg would use it).
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "data" {
		t.Fatalf("status %d body %q", resp.StatusCode, b)
	}
	if f.InputURL("../x") != "" {
		t.Fatal("InputURL accepted ../")
	}
	// Wrong token → ErrUnauthorized mentioning the token.
	bad, _ := New(Config{BaseURL: srv.URL, Token: "nope", Share: "films"})
	_, err = bad.Stat(context.Background(), "x")
	if !errors.Is(err, ErrUnauthorized) || !strings.Contains(err.Error(), "token") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Info(context.Background(), srv.URL, "nope", nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("info err = %v", err)
	}
	info, err := Info(context.Background(), srv.URL+"/agent/v1", token, nil)
	if err != nil || info.Hostname != "nas" || len(info.Shares) != 1 {
		t.Fatalf("info = %+v %v", info, err)
	}
}

func TestErrorMapping(t *testing.T) {
	a := newFake()
	a.add("x", []byte("x"))
	srv, f := newTestFS(t, a, "")
	ctx := context.Background()
	for code, want := range map[int]error{401: ErrUnauthorized, 403: ErrUnauthorized, 404: vfs.ErrNotExist, 500: vfs.ErrOffline, 503: vfs.ErrOffline, 429: vfs.ErrOffline, 400: vfs.ErrPath} {
		a.mu.Lock()
		a.status = code
		a.mu.Unlock()
		if _, err := f.Stat(ctx, "x"); !errors.Is(err, want) {
			t.Errorf("%d: stat err = %v; want %v", code, err, want)
		}
		if _, err := f.Open(ctx, "x"); !errors.Is(err, want) {
			t.Errorf("%d: open err = %v; want %v", code, err, want)
		}
		if err := f.Walk(ctx, "", func(vfs.Entry) error { return nil }); !errors.Is(err, want) {
			t.Errorf("%d: walk err = %v; want %v", code, err, want)
		}
	}
	a.mu.Lock()
	a.status = 0
	a.mu.Unlock()
	if _, err := f.Stat(ctx, "missing"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	other, _ := New(Config{BaseURL: srv.URL, Token: token, Share: "music"})
	if err := other.Ping(ctx); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("ping unknown share: %v", err)
	}
	// Network error.
	srv.Close()
	if err := f.Ping(ctx); !errors.Is(err, vfs.ErrOffline) {
		t.Fatalf("ping closed server: %v", err)
	}
	if _, err := New(Config{BaseURL: "ftp://x", Share: "s"}); err == nil {
		t.Fatal("bad scheme accepted")
	}
	if _, err := New(Config{BaseURL: "http://x"}); err == nil {
		t.Fatal("missing share accepted")
	}
}

func TestEventsReconnect(t *testing.T) {
	oldMin, oldMax := EventsMinBackoff, EventsMaxBackoff
	EventsMinBackoff, EventsMaxBackoff = 10*time.Millisecond, 50*time.Millisecond
	defer func() { EventsMinBackoff, EventsMaxBackoff = oldMin, oldMax }()

	var conns atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			writeErr(w, 401, "unauthorized")
			return
		}
		n := conns.Add(1)
		if n == 2 {
			writeErr(w, 503, "busy") // one failed reconnection attempt
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, ": ping\n\n")
		fmt.Fprintf(w, "event: change\ndata: {\"share\":\"music\",\"paths\":[\"a\"]}\n\n")
		fmt.Fprintf(w, "event: change\ndata: {\"share\":\"films\",\"paths\":[\"lib/a%d.mkv\",\"other/b.mkv\"]}\n\n", n)
		w.(http.Flusher).Flush()
		if n >= 4 {
			<-r.Context().Done() // stay connected
		}
		// otherwise: drop the connection
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	if _, err := Events(ctx, Config{BaseURL: srv.URL, Token: "bad", Share: "films"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad token: %v", err)
	}
	ch, err := Events(ctx, Config{BaseURL: srv.URL, Token: token, Share: "films", Root: "lib"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	timeout := time.After(5 * time.Second)
	for len(got) < 5 {
		select {
		case ev := <-ch:
			if ev.Share != "films" {
				t.Fatalf("event for other share: %+v", ev)
			}
			if ev.Resync {
				got = append(got, "resync")
				continue
			}
			got = append(got, strings.Join(ev.Paths, ","))
		case <-timeout:
			t.Fatalf("timeout; got %v", got)
		}
	}
	want := []string{"a1.mkv", "resync", "a3.mkv", "resync", "a4.mkv"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v; want %v", got, want)
	}
	cancel()
	select {
	case _, ok := <-ch:
		for ok {
			_, ok = <-ch
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after cancel")
	}
}

func TestEventsOfflineAtStart(t *testing.T) {
	oldMin := EventsMinBackoff
	EventsMinBackoff = 10 * time.Millisecond
	defer func() { EventsMinBackoff = oldMin }()
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	ch, err := Events(ctx, Config{BaseURL: addr, Token: token})
	if err != nil {
		t.Fatalf("offline agent should be retried, got %v", err)
	}
	for range ch {
	}
}

// BenchmarkReadThroughput reads a 200 MB file sequentially through agentfs.
func BenchmarkReadThroughput(b *testing.B) {
	a := newFake()
	data := make([]byte, 200<<20)
	a.add("big.bin", data)
	srv := httptest.NewServer(a)
	defer srv.Close()
	f, _ := New(Config{BaseURL: srv.URL, Token: token, Share: "films"})
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		fl, err := f.Open(context.Background(), "big.bin")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.CopyBuffer(io.Discard, fl, make([]byte, 32<<10)); err != nil {
			b.Fatal(err)
		}
		fl.Close()
	}
}
