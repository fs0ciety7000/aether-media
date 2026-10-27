package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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
	ts := httptest.NewServer(New(st, slog.New(slog.DiscardHandler)).Handler(web))
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
