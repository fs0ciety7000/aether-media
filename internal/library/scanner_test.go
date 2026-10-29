package library

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

func touch(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type fixture struct {
	t       *testing.T
	ctx     context.Context
	store   *store.Store
	scanner *Scanner
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.DiscardHandler)
	hub := events.NewHub()
	srcs, err := sources.NewForTests(st, t.TempDir(), hub, log)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, ctx: ctx, store: st, scanner: NewScanner(st, jobs.NewRunner(st, log), hub, log, srcs)}
}

func (f *fixture) library(kind string, roots ...string) int64 {
	f.t.Helper()
	lib, err := f.store.CreateLibrary(f.ctx, db.CreateLibraryParams{Name: kind, Kind: kind})
	if err != nil {
		f.t.Fatal(err)
	}
	for _, r := range roots {
		if _, err := f.store.AddLocalSource(f.ctx, lib.ID, r); err != nil {
			f.t.Fatal(err)
		}
	}
	return lib.ID
}

func (f *fixture) scan(lib int64) Result {
	f.t.Helper()
	res, err := f.scanner.Scan(f.ctx, lib, false)
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func (f *fixture) counts(lib int64) map[string]int64 {
	f.t.Helper()
	stats, err := f.store.LibraryStats(f.ctx, lib)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]int64{}
	for _, s := range stats {
		out[s.Kind] = s.N
	}
	return out
}

func (f *fixture) items(lib int64, kind string) []db.Item {
	f.t.Helper()
	items, err := f.store.ListItems(f.ctx, db.ListItemsParams{
		LibraryID: sql.NullInt64{Int64: lib, Valid: true}, Kind: sql.NullString{String: kind, Valid: true}, Lim: 1000,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return items
}

func TestScanMovies(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	touch(t, root,
		"Heat (1995)/Heat (1995) - 1080p.mkv",
		"Heat (1995)/Heat (1995) - 2160p.mkv",
		"Heat (1995)/Trailers/Heat trailer.mkv",
		"Heat (1995)/heat.nfo",
		"Kill Bill (2003)/Kill Bill (2003) - cd1.avi",
		"Kill Bill (2003)/Kill Bill (2003) - cd2.avi",
		"Inception.2010.1080p.BluRay.x264-GROUP.mkv",
		"Le Fabuleux Destin d'Amélie Poulain (2001)/Amelie.2001.1080p.BluRay.x264-GRP.mkv",
		"Sample/sample.mkv",
		".hidden/Hidden (2000).mkv",
		"Ignored/.aetherignore",
		"Ignored/Ignored (2001).mkv",
		"@eaDir/Thumb (2002).mkv",
		"Avatar (2009) {tmdb-19995}/Avatar (2009) [tmdbid-19995] - [BR-DISK]-GRP.ISO", // disc image: ignored
	)
	lib := f.library(KindMovies, root)

	res := f.scan(lib)
	if res.Added != 6 || res.Skipped != 1 {
		t.Errorf("first scan: %+v", res)
	}
	movies := f.items(lib, "movie")
	titles := map[string]int64{}
	for _, m := range movies {
		titles[m.Title] = m.Year.Int64
	}
	want := map[string]int64{"Heat": 1995, "Kill Bill": 2003, "Inception": 2010, "Le Fabuleux Destin d'Amélie Poulain": 2001}
	if len(titles) != len(want) {
		t.Fatalf("movies = %v, want %v", titles, want)
	}
	for k, v := range want {
		if titles[k] != v {
			t.Errorf("movie %q year = %d, want %d (all: %v)", k, titles[k], v, titles)
		}
	}
	for _, m := range movies {
		files, _ := f.store.ListItemMediaFiles(f.ctx, m.ID)
		if m.Title == "Kill Bill" && (len(files) != 2 || files[0].Part != 1 || files[1].Part != 2) {
			t.Errorf("Kill Bill parts = %+v", files)
		}
		if m.Title == "Heat" && len(files) != 2 {
			t.Errorf("Heat versions = %d, want 2", len(files))
		}
	}
	if stats, _ := f.store.JobStats(f.ctx); len(stats) != 1 || stats[0].N != 6 {
		t.Errorf("probe jobs = %+v, want 6 queued", stats)
	}

	// Rescan without changes is a no-op.
	if res := f.scan(lib); res.Unchanged != 6 || res.Added+res.Updated+res.Removed != 0 {
		t.Errorf("rescan: %+v", res)
	}

	// Deleting a file marks its movie missing; the other movies stay.
	os.Remove(filepath.Join(root, "Inception.2010.1080p.BluRay.x264-GROUP.mkv"))
	if res := f.scan(lib); res.Removed != 1 {
		t.Errorf("after delete: %+v", res)
	}
	if n := f.counts(lib)["movie"]; n != 3 {
		t.Errorf("movies after delete = %d, want 3", n)
	}
}

func TestScanUnavailableRootKeepsItems(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	touch(t, root, "Heat (1995)/Heat (1995).mkv")
	lib := f.library(KindMovies, root)
	f.scan(lib)

	// Simulate an unmounted share: the root vanishes entirely.
	moved := root + ".off"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(moved, root)
	res := f.scan(lib)
	if res.Removed != 0 || len(res.Unavailable) != 1 {
		t.Errorf("unavailable root: %+v", res)
	}
	if n := f.counts(lib)["movie"]; n != 1 {
		t.Errorf("movie must survive an unavailable root, got %d", n)
	}
}

func TestScanShows(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	touch(t, root,
		"Breaking Bad (2008)/Season 01/Breaking.Bad.S01E01.Pilot.1080p.mkv",
		"Breaking Bad (2008)/Season 01/Breaking.Bad.S01E02E03.1080p.mkv",
		"Breaking Bad (2008)/Season 02/Breaking Bad - S02E01 - Seven Thirty-Seven.mkv",
		"Breaking Bad (2008)/Specials/Breaking.Bad.S00E01.Good.Cop.Bad.Cop.mkv",
	)
	lib := f.library(KindShows, root)
	f.scan(lib)

	c := f.counts(lib)
	if c["series"] != 1 || c["season"] != 3 || c["episode"] != 4 {
		t.Fatalf("counts = %v", c)
	}
	series := f.items(lib, "series")[0]
	if series.Title != "Breaking Bad" || series.Year.Int64 != 2008 {
		t.Errorf("series = %q (%d)", series.Title, series.Year.Int64)
	}
	seasons, _ := f.store.ListItems(f.ctx, db.ListItemsParams{ParentID: sql.NullInt64{Int64: series.ID, Valid: true}, Lim: 10})
	if len(seasons) != 3 || seasons[0].Title != "Spéciaux" || seasons[1].IndexNumber.Int64 != 1 {
		t.Errorf("seasons = %+v", seasons)
	}
	eps, _ := f.store.ListItems(f.ctx, db.ListItemsParams{ParentID: sql.NullInt64{Int64: seasons[1].ID, Valid: true}, Lim: 10})
	if len(eps) != 2 || eps[0].IndexNumber.Int64 != 1 || eps[1].IndexNumber.Int64 != 2 || eps[1].IndexNumberEnd.Int64 != 3 {
		t.Errorf("season 1 episodes = %+v", eps)
	}

	// Removing a whole season folder flags the season missing, not the series.
	os.RemoveAll(filepath.Join(root, "Breaking Bad (2008)", "Season 02"))
	f.scan(lib)
	if c := f.counts(lib); c["season"] != 2 || c["series"] != 1 || c["episode"] != 3 {
		t.Errorf("after season removal: %v", c)
	}
}

func TestScanMusic(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	touch(t, root,
		"Daft Punk/Discovery (2001)/01 - One More Time.flac",
		"Daft Punk/Discovery (2001)/02 - Aerodynamic.flac",
		"Daft Punk/Alive 2007/CD2/01 - Human After All.mp3",
		"Daft Punk/Discovery (2001)/cover.jpg",
	)
	lib := f.library(KindMusic, root)
	f.scan(lib)
	if c := f.counts(lib); c["artist"] != 1 || c["album"] != 2 || c["track"] != 3 {
		t.Fatalf("counts = %v", c)
	}
	for _, tr := range f.items(lib, "track") {
		if tr.Title == "Human After All" && tr.ParentIndexNumber.Int64 != 2 {
			t.Errorf("disc folder not applied: %+v", tr)
		}
	}
}

func TestRescanPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	f := newFixture(t)
	root := t.TempDir()
	var rels []string
	for s := 1; s <= 20; s++ {
		for e := 1; e <= 25; e++ {
			rels = append(rels, fmt.Sprintf("Show %02d (2010)/Season %d/Show.S%02dE%02d.1080p.mkv", s, 1+e%3, 1+e%3, e))
		}
	}
	for m := 1; m <= 1500; m++ {
		rels = append(rels, fmt.Sprintf("Movie Number %d (%d)/Movie.Number.%d.%d.mkv", m, 1950+m%70, m, 1950+m%70))
	}
	touch(t, root, rels...)
	lib := f.library(KindMixed, root)
	start := time.Now()
	res := f.scan(lib)
	first := time.Since(start)
	if res.Added != len(rels) {
		t.Fatalf("added %d of %d (skipped %d)", res.Added, len(rels), res.Skipped)
	}
	start = time.Now()
	res = f.scan(lib)
	again := time.Since(start)
	if res.Unchanged != len(rels) || again > 5*time.Second {
		t.Errorf("rescan of %d files took %s: %+v", len(rels), again, res)
	}
	t.Logf("%d files: first scan %s, unchanged rescan %s", len(rels), first, again)
}
