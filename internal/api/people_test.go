package api

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/images"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

func TestPersonImageServedFromCache(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	img := image.NewRGBA(image.Rect(0, 0, 40, 60))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/w185/actor.png" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	}))
	defer remote.Close()

	// The test "remote" lives on loopback: allow it explicitly. A second
	// cache with the default policy refuses it (SSRF guard).
	cache, err := images.New(images.Options{Dir: t.TempDir(), AllowNetworks: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	if err != nil {
		t.Fatal(err)
	}
	pid, err := st.UpsertPerson(ctx, db.UpsertPersonParams{Name: "Audrey Tautou", Key: "tmdb:1", ProviderIds: "{}", ImageUrl: remote.URL + "/w185/actor.png"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(Deps{Store: st, Log: slog.New(slog.DiscardHandler), Images: cache}).Handler(http.NotFoundHandler()))
	defer ts.Close()
	c := loginAdmin(t, ts)

	u := personImageURL(pid, remote.URL+"/w185/actor.png")
	if !strings.HasPrefix(u, "/api/v1/people/"+strconv.FormatInt(pid, 10)+"/image?v=") {
		t.Fatalf("url %q", u)
	}
	for range 2 {
		res, err := c.Get(ts.URL + u + "&w=92")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "image/") {
			t.Fatalf("status %d %s", res.StatusCode, res.Header.Get("Content-Type"))
		}
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(body)); err != nil || cfg.Width != 40 {
			// Narrower than 92 px: served at its own size, never upscaled.
			t.Fatalf("decoded %+v %v", cfg, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetched %d times, want 1", hits.Load())
	}
	p, _ := st.GetPersonImage(ctx, pid)
	if len(p.ImageHash) != 64 {
		t.Fatalf("hash not stored: %+v", p)
	}
	// A new source URL resets the cached copy; the same URL keeps it.
	if _, err := st.UpsertPerson(ctx, db.UpsertPersonParams{Name: "Audrey Tautou", Key: "tmdb:1", ProviderIds: "{}", ImageUrl: remote.URL + "/w185/actor.png"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := st.GetPersonImage(ctx, pid); p.ImageHash == "" {
		t.Fatal("same URL dropped the cached hash")
	}
	if _, err := st.UpsertPerson(ctx, db.UpsertPersonParams{Name: "Audrey Tautou", Key: "tmdb:1", ProviderIds: "{}", ImageUrl: remote.URL + "/w185/missing.png"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := st.GetPersonImage(ctx, pid); p.ImageHash != "" {
		t.Fatal("new URL kept the old hash")
	}
	for range 2 { // a broken source is not fetched again right away
		res, err := c.Get(ts.URL + "/api/v1/people/" + strconv.FormatInt(pid, 10) + "/image")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 404 {
			t.Fatalf("missing photo: %d", res.StatusCode)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("remote fetched %d times, want 2", hits.Load())
	}

	// Default policy: a loopback photo URL (e.g. from an NFO) is never fetched.
	guarded, _ := images.New(images.Options{Dir: t.TempDir()})
	other, _ := st.UpsertPerson(ctx, db.UpsertPersonParams{Name: "X", Key: "name:x", ProviderIds: "{}", ImageUrl: remote.URL + "/w185/actor.png"})
	ts2 := httptest.NewServer(New(Deps{Store: st, Log: slog.New(slog.DiscardHandler), Images: guarded}).Handler(http.NotFoundHandler()))
	defer ts2.Close()
	res, err := c.Get(ts2.URL + "/api/v1/people/" + strconv.FormatInt(other, 10) + "/image")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 || hits.Load() != 2 {
		t.Fatalf("guarded: status %d, hits %d", res.StatusCode, hits.Load())
	}
	if personImageURL(1, "/srv/media/.actors/x.jpg") != "" || personImageURL(1, "") != "" {
		t.Fatal("non-http source must not get a URL")
	}
}
