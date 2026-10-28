package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "spa") })
	log := slog.New(slog.DiscardHandler)
	srcs, err := sources.NewForTests(st, t.TempDir(), events.NewHub(), log)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(Deps{Store: st, Log: log, Sources: srcs}).Handler(web))
	t.Cleanup(ts.Close)
	return ts
}

func TestSystemInfo(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Get(ts.URL + "/api/v1/system/info")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var info systemInfo
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || info.Name != "Aether" || !info.SetupRequired {
		t.Fatalf("got %d %+v", res.StatusCode, info)
	}
}

func TestRouting(t *testing.T) {
	ts := newTestServer(t)
	cases := map[string]int{
		"/healthz":     200,
		"/api/v1/nope": 404,
		"/films/123":   200, // SPA route
	}
	for path, want := range cases {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, res.StatusCode, want)
		}
	}
}

func TestCreateLibraryValidation(t *testing.T) {
	ts := newTestServer(t)
	dir := t.TempDir()
	post := func(body string) int {
		res, err := http.Post(ts.URL+"/api/v1/libraries", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	q := strconv.Quote
	cases := []struct {
		body string
		want int
	}{
		{`{"name":"Films","kind":"movies","paths":[` + q(dir) + `]}`, 201},
		{`{"name":"Encore","kind":"movies","paths":[` + q(dir) + `]}`, 409},                     // same folder
		{`{"name":"Sous","kind":"movies","paths":[` + q(filepath.Join(dir, "sub")) + `]}`, 409}, // nested in an existing library
		{`{"name":"Rel","kind":"movies","paths":["relative/path"]}`, 400},                       // not absolute
		{`{"name":"Photos","kind":"photos","paths":[` + q(t.TempDir()) + `]}`, 400},             // kind not supported yet
		{`{"name":"","kind":"movies","paths":[` + q(t.TempDir()) + `]}`, 400},                   // empty name
		{`not json`, 400},
	}
	for _, c := range cases {
		if got := post(c.body); got != c.want {
			t.Errorf("%s → %d, want %d", c.body, got, c.want)
		}
	}
	res, err := http.Get(ts.URL + "/api/v1/libraries")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var libs []libraryDTO
	if err := json.NewDecoder(res.Body).Decode(&libs); err != nil || len(libs) != 1 || libs[0].Paths[0] != dir {
		t.Fatalf("libraries = %+v, %v", libs, err)
	}
}
