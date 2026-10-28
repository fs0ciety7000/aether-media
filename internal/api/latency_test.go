package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// TestLatency20k measures browse and search latency on 20 000 synthetic
// items (PLAN M4: < 100 ms per API page). Opt-in, it takes ~20 s:
//
//	AETHER_LATENCY=1 go test ./internal/api -run Latency20k -v
//
// Every request is repeated; the p50 / p95 / max are logged and the test
// fails if a p95 exceeds 100 ms.
func TestLatency20k(t *testing.T) {
	if os.Getenv("AETHER_LATENCY") == "" {
		t.Skip("set AETHER_LATENCY=1")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "lat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	start := time.Now()
	libID := seed20k(t, st)
	t.Logf("seed: 20 000 items in %v", time.Since(start).Round(time.Millisecond))

	ts := httptest.NewServer(New(Deps{Store: st, Log: slog.New(slog.DiscardHandler)}).Handler(http.NotFoundHandler()))
	t.Cleanup(ts.Close)
	admin := loginAdmin(t, ts)
	// A member with parental control: the filtered queries are the costly ones.
	res, err := admin.Post(ts.URL+"/api/v1/users", "application/json",
		strings.NewReader(`{"name":"enfant","password":"motdepasse","maxRatingAge":12,"blockUnrated":true}`))
	if err != nil || res.StatusCode != 201 {
		t.Fatalf("create member: %v %v", err, res.Status)
	}
	res.Body.Close()
	member := login(t, ts, "enfant", "motdepasse")

	lib := fmt.Sprint(libID)
	type probe struct{ name, path string }
	probes := []probe{
		{"browse title (page 1)", "/api/v1/items?library=" + lib + "&limit=60"},
		{"browse dateAdded", "/api/v1/items?library=" + lib + "&sort=dateAdded&limit=60"},
		{"browse rating", "/api/v1/items?library=" + lib + "&sort=rating&limit=60"},
		{"browse random", "/api/v1/items?library=" + lib + "&sort=random&limit=60"},
		{"browse genre + years", "/api/v1/items?library=" + lib + "&genre=Drame&yearFrom=1990&yearTo=2015&limit=60"},
		{"browse 4K HDR", "/api/v1/items?library=" + lib + "&resolution=4K&hdr=true&limit=60"},
		{"browse unplayed", "/api/v1/items?library=" + lib + "&played=false&limit=60"},
		{"episodes of a season", "/api/v1/items?kind=episode&limit=60"},
		{"filters", "/api/v1/libraries/" + lib + "/filters"},
		{"home", "/api/v1/home"},
		{"search « ma »", "/api/v1/search?q=ma&limit=8"},
		{"search « nuit claire »", "/api/v1/search?q=nuit%20claire&limit=8"},
		{"search « zz » (none)", "/api/v1/search?q=zzq&limit=8"},
	}
	// Deep page: follow the cursor 100 pages in (item 6 000).
	cursorPath := func(c *http.Client) string {
		p := "/api/v1/items?library=" + lib + "&limit=60"
		cur := ""
		for range 100 {
			var page itemPage
			getJSONWith(t, c, ts.URL+p+"&cursor="+url.QueryEscape(cur), &page)
			cur = page.NextCursor
		}
		return p + "&cursor=" + url.QueryEscape(cur)
	}

	const runs = 25
	worst := time.Duration(0)
	for _, who := range []struct {
		name string
		c    *http.Client
	}{{"admin", admin}, {"membre (−12 ans)", member}} {
		all := append(slices.Clone(probes), probe{"browse page 100 (cursor)", cursorPath(who.c)})
		for _, p := range all {
			var ds []time.Duration
			for range runs {
				t0 := time.Now()
				res, err := who.c.Get(ts.URL + p.path)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, res.Body)
				res.Body.Close()
				if res.StatusCode != 200 {
					t.Fatalf("%s: %d", p.path, res.StatusCode)
				}
				ds = append(ds, time.Since(t0))
			}
			slices.Sort(ds)
			p50, p95, mx := ds[len(ds)/2], ds[len(ds)*95/100], ds[len(ds)-1]
			worst = max(worst, p95)
			t.Logf("| %s | %s | %.1f | %.1f | %.1f |", who.name, p.name, ms(p50), ms(p95), ms(mx))
			if p95 > 100*time.Millisecond {
				t.Errorf("%s / %s: p95 %v > 100 ms", who.name, p.name, p95)
			}
		}
	}
	t.Logf("worst p95: %.1f ms", ms(worst))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func login(t *testing.T, ts *httptest.Server, name, pass string) *http.Client {
	t.Helper()
	res, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", strings.NewReader(fmt.Sprintf(`{"name":%q,"password":%q}`, name, pass)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct{ Token string }
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || out.Token == "" {
		t.Fatalf("login %s: %d %v", name, res.StatusCode, err)
	}
	return &http.Client{Transport: bearerTransport{out.Token}}
}

func getJSONWith(t *testing.T, c *http.Client, u string, out any) {
	t.Helper()
	res, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

// seed20k writes 15 000 movies and 500 series × 10 episodes (one season
// each), with genres, cast, ratings, video streams and FTS rows.
func seed20k(t *testing.T, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	lib, err := st.CreateLibrary(ctx, db.CreateLibraryParams{Name: "Tout", Kind: "mixed"})
	if err != nil {
		t.Fatal(err)
	}
	src, err := st.AddLocalSource(ctx, lib.ID, "/media")
	if err != nil {
		t.Fatal(err)
	}
	words := strings.Fields("nuit claire marée basse orbite archipel terres été dernier brigade fleuve dune sel lanterne veilleur port écho colline rivage hiver lumière ombre ville silence voyage retour maison mer forêt étoile")
	genres := strings.Fields("Drame Comédie Action Thriller Romance Aventure Animation Policier Mystère Fantastique Horreur Reportage Famille Guerre Western Musique Histoire Science-fiction Téléfilm Biographie")
	ratings := []string{"Tous publics", "Déconseillé aux moins de 10 ans", "Déconseillé aux moins de 12 ans", "Déconseillé aux moins de 16 ans", "PG-13", "R", ""}
	r := rand.New(rand.NewPCG(1, 2))
	title := func() string {
		n := 1 + r.IntN(3)
		ws := make([]string, n)
		for i := range ws {
			ws[i] = words[r.IntN(len(words))]
		}
		return strings.ToUpper(ws[0][:1]) + strings.Join(ws, " ")[1:]
	}
	gids := make([]int64, len(genres))
	people := make([]int64, 400)
	err = st.Tx(ctx, func(q *db.Queries) error {
		for i, g := range genres {
			if gids[i], err = q.UpsertGenre(ctx, g); err != nil {
				return err
			}
		}
		for i := range people {
			if people[i], err = q.UpsertPerson(ctx, db.UpsertPersonParams{Name: fmt.Sprintf("Personne %d", i), Key: fmt.Sprintf("name:p%d", i), ProviderIds: "{}"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	decorate := func(q *db.Queries, id int64, path string, video bool) error {
		err := q.ApplyItemMetadata(ctx, db.ApplyItemMetadataParams{
			Overview:        sql.NullString{String: "Synopsis " + title(), Valid: true},
			RuntimeMs:       sql.NullInt64{Int64: int64(80+r.IntN(80)) * 60000, Valid: true},
			OfficialRating:  sql.NullString{String: ratings[r.IntN(len(ratings))], Valid: true},
			CommunityRating: sql.NullFloat64{Float64: 4 + r.Float64()*5, Valid: r.IntN(10) > 0},
			ProviderIds:     "{}", IdentifyStatus: "matched", ID: id,
		})
		if err != nil {
			return err
		}
		if err := store.UpdateRatingAge(ctx, q, id); err != nil {
			return err
		}
		for _, g := range r.Perm(len(gids))[:1+r.IntN(3)] {
			if err := q.AddItemGenre(ctx, db.AddItemGenreParams{ItemID: id, GenreID: gids[g]}); err != nil {
				return err
			}
		}
		for k := range 4 {
			if err := q.AddItemPerson(ctx, db.AddItemPersonParams{ItemID: id, PersonID: people[r.IntN(len(people))], Kind: "actor", Ord: int64(k)}); err != nil {
				return err
			}
		}
		if !video {
			return nil
		}
		mf, err := q.UpsertMediaFile(ctx, db.UpsertMediaFileParams{ItemID: id, LibraryID: lib.ID, SourceID: sql.NullInt64{Int64: src.ID, Valid: true}, Path: path, Size: 1 << 30, Mtime: 1})
		if err != nil {
			return err
		}
		w, h, hdr := int64(1920), int64(1080), ""
		switch r.IntN(10) {
		case 0:
			w, h, hdr = 3840, 2160, "HDR10"
		case 1, 2:
			w, h = 1280, 720
		}
		return q.InsertMediaStream(ctx, db.InsertMediaStreamParams{MediaFileID: mf.ID, Type: "video", Codec: "h264", Width: w, Height: h, Hdr: hdr})
	}
	const batch = 500
	for i := 0; i < 15000; i += batch {
		err := st.Tx(ctx, func(q *db.Queries) error {
			for j := i; j < i+batch; j++ {
				name := title()
				id, err := q.UpsertItem(ctx, db.UpsertItemParams{Key: sql.NullString{String: fmt.Sprintf("movie:%d", j), Valid: true}, LibraryID: lib.ID,
					Kind: "movie", Title: name, SortTitle: strings.ToLower(name), Year: sql.NullInt64{Int64: int64(1950 + r.IntN(75)), Valid: true}, ProviderIds: "{}"})
				if err != nil {
					return err
				}
				if err := decorate(q, id, fmt.Sprintf("/media/m%d.mkv", j), true); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for s := range 500 {
		err := st.Tx(ctx, func(q *db.Queries) error {
			name := title()
			sid, err := q.UpsertItem(ctx, db.UpsertItemParams{Key: sql.NullString{String: fmt.Sprintf("series:%d", s), Valid: true}, LibraryID: lib.ID,
				Kind: "series", Title: name, SortTitle: strings.ToLower(name), Year: sql.NullInt64{Int64: int64(1990 + r.IntN(35)), Valid: true}, ProviderIds: "{}"})
			if err != nil {
				return err
			}
			if err := decorate(q, sid, "", false); err != nil {
				return err
			}
			one := int64(1)
			season, err := q.UpsertItem(ctx, db.UpsertItemParams{Key: sql.NullString{String: fmt.Sprintf("season:%d", s), Valid: true}, LibraryID: lib.ID,
				ParentID: sql.NullInt64{Int64: sid, Valid: true}, Kind: "season", Title: "Saison 1", SortTitle: "0001", IndexNumber: sql.NullInt64{Int64: one, Valid: true}, ProviderIds: "{}"})
			if err != nil {
				return err
			}
			for e := range 10 {
				eid, err := q.UpsertItem(ctx, db.UpsertItemParams{Key: sql.NullString{String: fmt.Sprintf("episode:%d:%d", s, e), Valid: true}, LibraryID: lib.ID,
					ParentID: sql.NullInt64{Int64: season, Valid: true}, Kind: "episode", Title: title(), SortTitle: fmt.Sprintf("0001-%05d", e+1),
					IndexNumber: sql.NullInt64{Int64: int64(e + 1), Valid: true}, ParentIndexNumber: sql.NullInt64{Int64: 1, Valid: true}, ProviderIds: "{}"})
				if err != nil {
					return err
				}
				if err := decorate(q, eid, fmt.Sprintf("/media/s%d/e%d.mkv", s, e), true); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.DB.ExecContext(ctx, "UPDATE items SET date_added = unixepoch() - abs(random() % 94608000)"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, "ANALYZE"); err != nil {
		t.Fatal(err)
	}
	return lib.ID
}
