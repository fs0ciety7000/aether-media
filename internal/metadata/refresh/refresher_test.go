package refresh

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/library"
	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// fakeRemote serves canned results keyed by tmdb id.
type fakeRemote struct {
	enabled bool
	cands   map[string][]metadata.Candidate // title → candidates
	results map[string]*metadata.Result     // kind:id[:season] → result
	fetches int
}

func (f *fakeRemote) Name() string  { return "fake" }
func (f *fakeRemote) Enabled() bool { return f.enabled }
func (f *fakeRemote) Supports(kind string) bool {
	return kind != metadata.KindArtist && kind != metadata.KindAlbum
}
func (f *fakeRemote) Search(_ context.Context, q metadata.Query) ([]metadata.Candidate, error) {
	return f.cands[q.Title], nil
}
func (f *fakeRemote) Fetch(_ context.Context, q metadata.Query) (*metadata.Result, error) {
	f.fetches++
	key := q.Kind + ":" + q.ProviderIDs["tmdb"]
	if q.Kind == metadata.KindSeason {
		key = fmt.Sprintf("season:%s:%d", q.ParentIDs["tmdb"], q.IndexNumber)
	}
	if r, ok := f.results[key]; ok {
		return r, nil
	}
	return nil, metadata.ErrNotFound
}

type fakeLocal struct{ byTitle map[string]*metadata.Result }

func (fakeLocal) Name() string         { return "local" }
func (fakeLocal) Supports(string) bool { return true }
func (f fakeLocal) Local(_ context.Context, q metadata.Query) (*metadata.Result, error) {
	return f.byTitle[q.Title], nil
}

type env struct {
	t      *testing.T
	ctx    context.Context
	st     *store.Store
	r      *Refresher
	remote *fakeRemote
	lib    int64
}

func setup(t *testing.T, kind string, files ...string) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	log := slog.New(slog.DiscardHandler)
	runner := jobs.NewRunner(st, log)
	hub := events.NewHub()
	lib, _ := st.CreateLibrary(ctx, db.CreateLibraryParams{Name: "L", Kind: kind})
	st.AddLibraryPath(ctx, db.AddLibraryPathParams{LibraryID: lib.ID, Path: root})
	if _, err := library.NewScanner(st, runner, hub, log).Scan(ctx, lib.ID, false); err != nil {
		t.Fatal(err)
	}
	remote := &fakeRemote{enabled: true, cands: map[string][]metadata.Candidate{}, results: map[string]*metadata.Result{}}
	ingest := func(_ context.Context, src string) (ImageInfo, error) {
		return ImageInfo{Hash: fmt.Sprintf("%064x", len(src)), Width: 500, Height: 750, Blurhash: "LEHV6nWB2yk8", Dominant: "#5ec4c0"}, nil
	}
	r := New(st, runner, hub, log, Config{Language: "fr-FR"}, []metadata.LocalProvider{fakeLocal{byTitle: map[string]*metadata.Result{}}}, []metadata.RemoteProvider{remote}, ingest)
	return &env{t: t, ctx: ctx, st: st, r: r, remote: remote, lib: lib.ID}
}

func (e *env) item(kind string) db.Item {
	e.t.Helper()
	items, err := e.st.ListItems(e.ctx, db.ListItemsParams{LibraryID: sql.NullInt64{Int64: e.lib, Valid: true}, Kind: sql.NullString{String: kind, Valid: true}, Lim: 10})
	if err != nil || len(items) == 0 {
		e.t.Fatalf("no %s: %v", kind, err)
	}
	return items[0]
}

func (e *env) run(itemID int64) {
	e.t.Helper()
	payload, _ := json.Marshal(refreshPayload{ItemID: itemID})
	if err := e.r.HandleRefresh(e.ctx, jobs.Job{Kind: JobRefresh, Payload: payload}); err != nil {
		e.t.Fatal(err)
	}
}

func TestRefreshMovieMatch(t *testing.T) {
	e := setup(t, "movies", "Amelie (2001)/Amelie.2001.mkv")
	e.remote.cands["Amelie"] = []metadata.Candidate{
		{ProviderIDs: metadata.ProviderIDs{"tmdb": "194"}, Title: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001, Score: 0.93},
		{ProviderIDs: metadata.ProviderIDs{"tmdb": "1"}, Title: "Amélie", Year: 2015, Score: 0.6},
	}
	e.remote.results["movie:194"] = &metadata.Result{
		Title: "Le Fabuleux Destin d'Amélie Poulain", Overview: "Amélie…", PremiereDate: "2001-04-25",
		Genres: []string{"Comédie", "Romance"}, ProviderIDs: metadata.ProviderIDs{"tmdb": "194", "imdb": "tt0211915"},
		People: []metadata.Person{{Name: "Audrey Tautou", Kind: "actor", Role: "Amélie", ProviderIDs: metadata.ProviderIDs{"tmdb": "3"}}, {Name: "Jean-Pierre Jeunet", Kind: "director"}},
		Images: []metadata.RemoteImage{{Type: "poster", URL: "https://image.tmdb.org/p.jpg"}, {Type: "backdrop", URL: "https://image.tmdb.org/b.jpg"}},
	}
	m := e.item("movie")
	e.run(m.ID)

	got, _ := e.st.GetItem(e.ctx, m.ID)
	if got.IdentifyStatus != StatusMatched || got.Title != "Le Fabuleux Destin d'Amélie Poulain" || got.Overview.String != "Amélie…" {
		t.Fatalf("item = %+v", got)
	}
	if ids := parseIDs(got.ProviderIds); ids["tmdb"] != "194" || ids["imdb"] != "tt0211915" {
		t.Errorf("ids = %v", ids)
	}
	if g, _ := e.st.ListItemGenres(e.ctx, m.ID); len(g) != 2 {
		t.Errorf("genres = %v", g)
	}
	if p, _ := e.st.ListItemPeople(e.ctx, m.ID); len(p) != 2 || p[0].Kind != "director" {
		t.Errorf("people = %+v", p)
	}
	imgs, _ := e.st.ListItemImages(e.ctx, m.ID)
	if len(imgs) != 2 {
		t.Fatalf("images = %+v", imgs)
	}
	payload, _ := json.Marshal(imagePayload{ImageID: imgs[0].ID})
	if err := e.r.HandleImage(e.ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if img, _ := e.st.GetImage(e.ctx, imgs[0].ID); !img.Hash.Valid || img.Dominant != "#5ec4c0" {
		t.Errorf("image not fetched: %+v", img)
	}

	// A second refresh uses the stored id without searching.
	e.remote.cands = nil
	e.run(m.ID)
	if got, _ := e.st.GetItem(e.ctx, m.ID); got.IdentifyStatus != StatusMatched {
		t.Errorf("status after second refresh = %s", got.IdentifyStatus)
	}
}

func TestRefreshAmbiguousIsUnmatched(t *testing.T) {
	e := setup(t, "movies", "Heat (1995)/Heat.mkv")
	e.remote.cands["Heat"] = []metadata.Candidate{
		{ProviderIDs: metadata.ProviderIDs{"tmdb": "949"}, Title: "Heat", Year: 1995, Score: 0.9},
		{ProviderIDs: metadata.ProviderIDs{"tmdb": "950"}, Title: "Heat", Year: 1995, Score: 0.89},
	}
	m := e.item("movie")
	e.run(m.ID)
	if got, _ := e.st.GetItem(e.ctx, m.ID); got.IdentifyStatus != StatusUnmatched {
		t.Fatalf("status = %s", got.IdentifyStatus)
	}
	// The user picks one: identify pins it and the refresh fetches it.
	e.remote.results["movie:949"] = &metadata.Result{Title: "Heat", Overview: "Los Angeles…"}
	if err := e.r.Identify(e.ctx, m.ID, metadata.ProviderIDs{"tmdb": "949"}); err != nil {
		t.Fatal(err)
	}
	e.run(m.ID)
	if got, _ := e.st.GetItem(e.ctx, m.ID); got.IdentifyStatus != StatusManual || got.Overview.String != "Los Angeles…" {
		t.Fatalf("after identify: %+v", got)
	}
}

func TestRefreshLockedAndLocal(t *testing.T) {
	e := setup(t, "movies", "Heat (1995)/Heat.mkv")
	e.remote.enabled = false
	e.r.local = []metadata.LocalProvider{fakeLocal{byTitle: map[string]*metadata.Result{
		"Mon titre": {Title: "Heat (NFO)", Overview: "Depuis le NFO"},
	}}}
	m := e.item("movie")
	locked, _ := json.Marshal([]string{"title"})
	e.st.UpdateItemFields(e.ctx, db.UpdateItemFieldsParams{Title: sql.NullString{String: "Mon titre", Valid: true}, LockedFields: string(locked), ID: m.ID})
	e.run(m.ID)
	got, _ := e.st.GetItem(e.ctx, m.ID)
	if got.Title != "Mon titre" || got.Overview.String != "Depuis le NFO" || got.IdentifyStatus != StatusLocal {
		t.Fatalf("got %+v", got)
	}
	if e.remote.fetches != 0 {
		t.Error("disabled provider must not be called")
	}
}

func TestRefreshSeriesCascade(t *testing.T) {
	e := setup(t, "shows",
		"Lupin (2021)/Saison 1/Lupin.S01E01.mkv",
		"Lupin (2021)/Saison 1/Lupin.S01E02.mkv",
	)
	e.remote.cands["Lupin"] = []metadata.Candidate{{ProviderIDs: metadata.ProviderIDs{"tmdb": "96677"}, Title: "Lupin", Year: 2021, Score: 0.97}}
	e.remote.results["series:96677"] = &metadata.Result{Title: "Lupin", Overview: "Assane Diop…", Status: "continuing"}
	e.remote.results["season:96677:1"] = &metadata.Result{Title: "Partie 1", Children: map[int]*metadata.Result{
		1: {Title: "Chapitre 1", Overview: "Le collier"}, 2: {Title: "Chapitre 2"},
	}}
	series := e.item("series")
	e.run(series.ID)
	if got, _ := e.st.GetItem(e.ctx, series.ID); got.Status.String != "continuing" {
		t.Fatalf("series = %+v", got)
	}
	// The series refresh queued its season.
	stats, _ := e.st.JobStats(e.ctx)
	queued := false
	for _, s := range stats {
		queued = queued || s.Kind == JobRefresh
	}
	if !queued {
		t.Fatal("season refresh not queued")
	}
	season := e.item("season")
	e.run(season.ID)
	eps, _ := e.st.ListChildItems(e.ctx, sql.NullInt64{Int64: season.ID, Valid: true})
	if len(eps) != 2 || eps[0].Title != "Chapitre 1" || eps[0].Overview.String != "Le collier" || eps[1].IdentifyStatus != StatusMatched {
		t.Fatalf("episodes = %+v", eps)
	}
	if need, _ := e.st.ListItemsNeedingRefresh(e.ctx, e.lib); len(need) != 0 {
		t.Errorf("still needing refresh: %+v", need)
	}
}
