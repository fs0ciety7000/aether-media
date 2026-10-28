package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/library"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// browseServer scans a small movie library and decorates it with genres,
// ratings, a person and a video stream.
func browseServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	for _, f := range []string{
		"Le Fabuleux Destin d'Amélie Poulain (2001)/amelie.mkv",
		"Heat (1995)/heat.mkv",
		"Inception (2010)/inception.mkv",
		"Ratatouille (2007)/ratatouille.mkv",
	} {
		p := filepath.Join(root, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(f), 0o644)
	}
	log := slog.New(slog.DiscardHandler)
	runner := jobs.NewRunner(st, log)
	lib, _ := st.CreateLibrary(ctx, db.CreateLibraryParams{Name: "Films", Kind: "movies"})
	st.AddLibraryPath(ctx, db.AddLibraryPathParams{LibraryID: lib.ID, Path: root})
	if _, err := library.NewScanner(st, runner, events.NewHub(), log).Scan(ctx, lib.ID, false); err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]db.Item{}
	items, _ := st.ListItems(ctx, db.ListItemsParams{Lim: 100})
	for _, it := range items {
		byTitle[it.Title] = it
	}
	genre := func(title string, genres ...string) {
		for _, g := range genres {
			gid, _ := st.UpsertGenre(ctx, g)
			st.AddItemGenre(ctx, db.AddItemGenreParams{ItemID: byTitle[title].ID, GenreID: gid})
		}
	}
	genre("Le Fabuleux Destin d'Amélie Poulain", "Comédie", "Romance")
	genre("Ratatouille", "Comédie", "Animation")
	genre("Heat", "Action", "Crime")
	genre("Inception", "Action", "Science-Fiction")
	for title, rating := range map[string]float64{"Heat": 8.3, "Inception": 8.8, "Ratatouille": 7.8} {
		st.DB.Exec("UPDATE items SET community_rating = ? WHERE id = ?", rating, byTitle[title].ID)
	}
	pid, _ := st.UpsertPerson(ctx, db.UpsertPersonParams{Name: "Audrey Tautou", Key: "name:audrey tautou", ProviderIds: "{}"})
	st.AddItemPerson(ctx, db.AddItemPersonParams{ItemID: byTitle["Le Fabuleux Destin d'Amélie Poulain"].ID, PersonID: pid, Kind: "actor"})
	// Inception gets a 4K HDR video stream.
	files, _ := st.ListItemMediaFiles(ctx, byTitle["Inception"].ID)
	st.InsertMediaStream(ctx, db.InsertMediaStreamParams{MediaFileID: files[0].ID, Type: "video", Codec: "hevc", Width: 3840, Height: 1600, Hdr: "HDR10"})

	ts := httptest.NewServer(New(Deps{Store: st, Log: log}).Handler(http.NotFoundHandler()))
	t.Cleanup(ts.Close)
	return ts
}

func getJSON(t *testing.T, ts *httptest.Server, path string, out any) int {
	t.Helper()
	res, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if out != nil && res.StatusCode == 200 {
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("%s: %v (%s)", path, err, body)
		}
	}
	return res.StatusCode
}

func titles(items []itemDTO) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

func TestBrowseFiltersAndSort(t *testing.T) {
	ts := browseServer(t)
	var page itemPage
	getJSON(t, ts, "/api/v1/items", &page)
	if page.Total != 4 || page.Items[0].Title != "Le Fabuleux Destin d'Amélie Poulain" {
		t.Fatalf("default listing: total %d, %v", page.Total, titles(page.Items))
	}
	getJSON(t, ts, "/api/v1/items?genre=comédie", &page)
	if page.Total != 2 {
		t.Errorf("genre filter: %v", titles(page.Items))
	}
	getJSON(t, ts, "/api/v1/items?sort=rating", &page)
	if got := titles(page.Items); got[0] != "Inception" || got[3] != "Le Fabuleux Destin d'Amélie Poulain" {
		t.Errorf("rating desc (nulls last): %v", got)
	}
	getJSON(t, ts, "/api/v1/items?yearFrom=2000&yearTo=2008&sort=year&order=asc", &page)
	if got := titles(page.Items); len(got) != 2 || got[0] != "Le Fabuleux Destin d'Amélie Poulain" {
		t.Errorf("year range: %v", got)
	}
	getJSON(t, ts, "/api/v1/items?resolution=4K&hdr=true", &page)
	if page.Total != 1 || page.Items[0].Video == nil || page.Items[0].Video.Resolution != "4K" || page.Items[0].Video.HDR != "HDR10" {
		t.Errorf("4K HDR: %+v", page.Items)
	}

	// Cursor pagination covers every item exactly once, also in random order.
	for _, sortKey := range []string{"title", "random"} {
		seen := map[int64]bool{}
		cursor := ""
		for range 10 {
			var p itemPage
			getJSON(t, ts, "/api/v1/items?limit=3&sort="+sortKey+"&cursor="+cursor, &p)
			for _, it := range p.Items {
				if seen[it.ID] {
					t.Fatalf("%s: duplicate %q", sortKey, it.Title)
				}
				seen[it.ID] = true
			}
			if cursor = p.NextCursor; cursor == "" {
				break
			}
		}
		if len(seen) != 4 {
			t.Errorf("%s pagination saw %d items", sortKey, len(seen))
		}
	}
	for _, bad := range []string{"sort=nope", "resolution=8K", "limit=0", "order=up", "cursor=@@"} {
		if code := getJSON(t, ts, "/api/v1/items?"+bad, nil); code != 400 {
			t.Errorf("%s → %d", bad, code)
		}
	}
}

func TestSearch(t *testing.T) {
	ts := browseServer(t)
	var res struct {
		Results struct {
			Movie  []itemDTO      `json:"movie"`
			Person []personHitDTO `json:"person"`
			Series []itemDTO      `json:"series"`
		} `json:"results"`
	}
	// Accent-insensitive prefix search.
	getJSON(t, ts, "/api/v1/search?q="+url.QueryEscape("ameli"), &res)
	if len(res.Results.Movie) != 1 || res.Results.Series == nil {
		t.Fatalf("ameli: %+v", res.Results)
	}
	getJSON(t, ts, "/api/v1/search?q="+url.QueryEscape("tautou"), &res)
	if len(res.Results.Person) != 1 || res.Results.Person[0].Count != 1 {
		t.Errorf("people: %+v", res.Results.Person)
	}
	// FTS syntax in user input must not break the query.
	for _, q := range []string{`"`, `he*at OR`, `NEAR(`, "a"} {
		if code := getJSON(t, ts, "/api/v1/search?q="+url.QueryEscape(q), &res); code != 200 {
			t.Errorf("%q → %d", q, code)
		}
	}
}

func TestHomeAndSimilar(t *testing.T) {
	ts := browseServer(t)
	var home homeDTO
	if code := getJSON(t, ts, "/api/v1/home", &home); code != 200 {
		t.Fatalf("home → %d", code)
	}
	if len(home.RecentlyAdded) != 1 || len(home.RecentlyAdded[0].Items) != 4 || len(home.Categories) == 0 || home.Categories[0].Genre != "Action" && home.Categories[0].Genre != "Comédie" {
		t.Errorf("home: %+v", home)
	}
	var page itemPage
	getJSON(t, ts, "/api/v1/items?genre=Animation", &page)
	var sim []itemDTO
	getJSON(t, ts, "/api/v1/items/"+itoa(page.Items[0].ID)+"/similar", &sim)
	if len(sim) != 1 || sim[0].Title != "Le Fabuleux Destin d'Amélie Poulain" {
		t.Errorf("similar to Ratatouille: %v", titles(sim))
	}
	var detail itemDetail
	getJSON(t, ts, "/api/v1/items/"+itoa(page.Items[0].ID), &detail)
	if len(detail.Genres) != 2 || detail.Ancestors == nil {
		t.Errorf("detail: %+v", detail)
	}
	var f filtersDTO
	getJSON(t, ts, "/api/v1/libraries/1/filters", &f)
	if len(f.Genres) != 6 || f.Years == nil || f.Years.Min != 1995 || !f.HasHDR || len(f.Resolutions) != 1 {
		t.Errorf("filters: %+v", f)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
