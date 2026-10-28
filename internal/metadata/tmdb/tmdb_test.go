package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/metadata/httpx"
)

const (
	testKey   = "0123456789abcdef0123456789abcdef"
	testToken = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ0ZXN0In0.c2lnbmF0dXJl"
	imgBase   = "https://img.test/t/p/"
)

// fakeTMDB serves testdata fixtures and records requests.
type fakeTMDB struct {
	t    *testing.T
	srv  *httptest.Server
	key  string
	mu   sync.Mutex
	reqs []*http.Request
}

func newFake(t *testing.T, key string) *fakeTMDB {
	f := &fakeTMDB{t: t, key: key}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTMDB) requests(pathPrefix string) []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*http.Request
	for _, r := range f.reqs {
		if strings.HasPrefix(r.URL.Path, pathPrefix) {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeTMDB) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r.Clone(context.Background()))
	f.mu.Unlock()

	q := r.URL.Query()
	if q.Get("api_key") != f.key && r.Header.Get("Authorization") != "Bearer "+f.key {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status_code":7,"status_message":"Invalid API key: You must be granted a valid key.","success":false}`))
		return
	}
	en := q.Get("language") == "en-US"
	file := ""
	switch p := strings.TrimPrefix(r.URL.Path, "/3"); p {
	case "/search/movie":
		switch q.Get("query") {
		case "Amélie":
			file = "search_movie_amelie.json"
		case "1917":
			file = "search_movie_1917.json"
		case "Retry Movie":
			if q.Get("year") == "" && q.Get("primary_release_year") == "" {
				file = "search_movie_amelie.json"
			}
		}
	case "/search/tv":
		if q.Get("query") == "Breaking Bad" {
			file = "search_tv_breaking_bad.json"
		}
	case "/find/tt0211915":
		if q.Get("external_source") == "imdb_id" {
			file = "find_tt0211915.json"
		}
	case "/find/81189":
		if q.Get("external_source") == "tvdb_id" {
			file = "find_tvdb_81189.json"
		}
	case "/movie/194":
		file = "movie_194.json"
	case "/movie/24":
		file = "movie_24.json"
		if en {
			file = "movie_24_en.json"
		}
	case "/tv/1396":
		file = "tv_1396.json"
	case "/tv/1396/season/1":
		file = "tv_1396_season_1.json"
		if en {
			file = "tv_1396_season_1_en.json"
		}
	default:
		if strings.HasPrefix(p, "/find/") {
			w.Write([]byte(`{"movie_results":[],"tv_results":[],"person_results":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status_code":34,"status_message":"The resource you requested could not be found.","success":false}`))
		return
	}
	if file == "" {
		w.Write([]byte(`{"page":1,"results":[],"total_pages":0,"total_results":0}`))
		return
	}
	b, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		f.t.Errorf("fixture: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.Write(b)
}

func newProvider(t *testing.T, key string, cacheDir string) (*Provider, *fakeTMDB) {
	t.Helper()
	f := newFake(t, testKey)
	c := httpx.New(httpx.Options{UserAgent: "Aether/test", CacheDir: cacheDir})
	p := New(Config{APIKey: key, BaseURL: f.srv.URL + "/3", ImageBaseURL: imgBase}, c)
	return p, f
}

func ids(cs []metadata.Candidate) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.ProviderIDs[metadata.IDTmdb])
	}
	return out
}

func TestDisabled(t *testing.T) {
	p, f := newProvider(t, "", "")
	if p.Enabled() || p.Name() != "tmdb" {
		t.Fatal("expected disabled tmdb provider")
	}
	ctx := context.Background()
	if _, err := p.Search(ctx, metadata.Query{Kind: metadata.KindMovie, Title: "Amélie"}); !errors.Is(err, metadata.ErrUnavailable) {
		t.Fatalf("Search: %v", err)
	}
	if _, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"tmdb": "194"}}); !errors.Is(err, metadata.ErrUnavailable) {
		t.Fatalf("Fetch: %v", err)
	}
	if n := len(f.requests("/")); n != 0 {
		t.Fatalf("%d requests made while disabled", n)
	}
}

func TestSupports(t *testing.T) {
	p := New(Config{APIKey: "x"}, httpx.New(httpx.Options{UserAgent: "t"}))
	for _, k := range []string{metadata.KindMovie, metadata.KindSeries, metadata.KindSeason, metadata.KindEpisode} {
		if !p.Supports(k) {
			t.Errorf("should support %s", k)
		}
	}
	if p.Supports(metadata.KindAlbum) {
		t.Error("should not support album")
	}
}

func TestAuthModes(t *testing.T) {
	ctx := context.Background()
	q := metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"tmdb": "194"}}

	p, f := newProvider(t, testKey, "")
	if _, err := p.Fetch(ctx, q); err != nil {
		t.Fatal(err)
	}
	r := f.requests("/3/movie/194")[0]
	if r.URL.Query().Get("api_key") != testKey || r.Header.Get("Authorization") != "" {
		t.Fatalf("v3 key: query=%v auth=%q", r.URL.Query(), r.Header.Get("Authorization"))
	}

	f2 := newFake(t, testToken)
	p2 := New(Config{APIKey: testToken, BaseURL: f2.srv.URL + "/3"}, httpx.New(httpx.Options{UserAgent: "t"}))
	if _, err := p2.Fetch(ctx, q); err != nil {
		t.Fatal(err)
	}
	r = f2.requests("/3/movie/194")[0]
	if r.URL.Query().Has("api_key") || r.Header.Get("Authorization") != "Bearer "+testToken {
		t.Fatalf("v4 token: query=%v auth=%q", r.URL.Query(), r.Header.Get("Authorization"))
	}
}

func TestInvalidKey(t *testing.T) {
	p, _ := newProvider(t, "wrongkey123", "")
	_, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindMovie, Title: "Amélie"})
	if !errors.Is(err, metadata.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "wrongkey123") {
		t.Fatalf("API key leaked in error: %v", err)
	}
}

func TestSearchMovieAmelie(t *testing.T) {
	p, f := newProvider(t, testKey, "")
	cs, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindMovie, Title: "Amélie", Year: 2001})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 || cs[0].ProviderIDs["tmdb"] != "194" {
		t.Fatalf("order = %v", ids(cs))
	}
	c := cs[0]
	if c.Provider != "tmdb" || c.Year != 2001 || c.Title != "Le Fabuleux Destin d'Amélie Poulain" ||
		c.ImageURL != imgBase+"w342/vZ9NhNbQQ3yhtiC5sbhpy5KTXns.jpg" {
		t.Fatalf("candidate = %+v", c)
	}
	for i := 1; i < len(cs); i++ {
		if cs[i].Score > cs[i-1].Score || cs[i].Score < 0 || cs[i].Score > 1 {
			t.Fatalf("scores not sorted/in range: %+v", cs)
		}
	}
	q := f.requests("/3/search/movie")[0].URL.Query()
	if q.Get("year") != "2001" || q.Get("language") != "fr-FR" || q.Get("include_adult") != "false" {
		t.Fatalf("search params = %v", q)
	}
}

func TestSearchMovie1917(t *testing.T) {
	p, _ := newProvider(t, testKey, "")
	ctx := context.Background()
	cs, err := p.Search(ctx, metadata.Query{Kind: metadata.KindMovie, Title: "1917", Year: 2019})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(cs); strings.Join(got, ",") != "530915,661914,403277" {
		t.Fatalf("order = %v (%+v)", got, cs)
	}
	if cs[0].Score < 0.95 {
		t.Fatalf("exact title+year should score high: %v", cs[0].Score)
	}
	// Without a year, the popular exact title still wins and scores ~1.
	cs, err = p.Search(ctx, metadata.Query{Kind: metadata.KindMovie, Title: "1917"})
	if err != nil {
		t.Fatal(err)
	}
	if cs[0].ProviderIDs["tmdb"] != "530915" || cs[0].Score < 0.95 {
		t.Fatalf("no year: %+v", cs[0])
	}
}

func TestScoreExactBeatsPopularNearMiss(t *testing.T) {
	q := metadata.Query{Title: "The Ring", Year: 2002}
	exact := score(q, "The Ring", "The Ring", 2002, 10, 100)
	near := score(q, "The Rings", "Rings", 2017, 100, 100)
	wrongYear := score(q, "The Ring", "Ringu", 1998, 50, 100)
	if !(exact > wrongYear && wrongYear > near) {
		t.Fatalf("exact=%v wrongYear=%v near=%v", exact, wrongYear, near)
	}
	offByOne := score(q, "The Ring", "", 2003, 10, 100)
	if !(exact > offByOne && offByOne > wrongYear) {
		t.Fatalf("±1 year: exact=%v off=%v wrong=%v", exact, offByOne, wrongYear)
	}
	// Original title counts as much as the localized one.
	if s := score(metadata.Query{Title: "Ringu", Year: 1998}, "The Ring", "Ringu", 1998, 0, 0); s != 1 {
		t.Fatalf("original title match = %v", s)
	}
}

func TestSearchRetriesWithoutYear(t *testing.T) {
	p, f := newProvider(t, testKey, "")
	cs, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindMovie, Title: "Retry Movie", Year: 1999})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) == 0 {
		t.Fatal("expected results from the year-less retry")
	}
	reqs := f.requests("/3/search/movie")
	if len(reqs) != 3 {
		t.Fatalf("requests = %d, want 3", len(reqs))
	}
	if reqs[0].URL.Query().Get("year") != "1999" || reqs[1].URL.Query().Get("primary_release_year") != "1999" ||
		reqs[2].URL.Query().Has("year") || reqs[2].URL.Query().Has("primary_release_year") {
		t.Fatalf("unexpected attempts: %v / %v / %v", reqs[0].URL, reqs[1].URL, reqs[2].URL)
	}
}

func TestSearchMovieByImdb(t *testing.T) {
	p, f := newProvider(t, testKey, "")
	cs, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindMovie, Title: "whatever",
		ProviderIDs: metadata.ProviderIDs{"imdb": "tt0211915"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].ProviderIDs["tmdb"] != "194" || cs[0].Score != 1 {
		t.Fatalf("got %+v", cs)
	}
	if len(f.requests("/3/search")) != 0 {
		t.Fatal("title search should not run when /find matches")
	}
}

func TestSearchMovieByTmdbID(t *testing.T) {
	p, _ := newProvider(t, testKey, "")
	cs, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"tmdb": "194"}})
	if err != nil || len(cs) != 1 || cs[0].Score != 1 || cs[0].Year != 2001 {
		t.Fatalf("got %+v, %v", cs, err)
	}
}

func TestFetchMovie(t *testing.T) {
	p, f := newProvider(t, testKey, "")
	r, err := p.Fetch(context.Background(), metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"tmdb": "194"}})
	if err != nil {
		t.Fatal(err)
	}
	q := f.requests("/3/movie/194")[0].URL.Query()
	if q.Get("append_to_response") != "credits,images,release_dates,external_ids" || q.Get("include_image_language") != "fr,en,null" {
		t.Fatalf("params = %v", q)
	}
	if r.Title != "Le Fabuleux Destin d'Amélie Poulain" || r.OriginalTitle != r.Title || r.Tagline != "Elle va changer votre vie." ||
		!strings.HasPrefix(r.Overview, "Amélie, une jeune serveuse") {
		t.Fatalf("texts: %+v", r)
	}
	if r.PremiereDate != "2001-04-25" || r.Year != 2001 || r.RuntimeMs != 122*60_000 || r.CommunityRating != 7.9 {
		t.Fatalf("dates/runtime/rating: %+v", r)
	}
	if r.OfficialRating != "U" {
		t.Fatalf("certification = %q, want FR theatrical U", r.OfficialRating)
	}
	if strings.Join(r.Genres, "|") != "Comédie|Romance" || strings.Join(r.Studios, "|") != "Claudie Ossard Productions|France 3 Cinéma|UGC" {
		t.Fatalf("genres/studios: %v %v", r.Genres, r.Studios)
	}
	if r.ProviderIDs["tmdb"] != "194" || r.ProviderIDs["imdb"] != "tt0211915" || r.Collection != "" {
		t.Fatalf("ids: %v collection %q", r.ProviderIDs, r.Collection)
	}
	// People.
	var actors, crew []string
	for _, pr := range r.People {
		if pr.Kind == "actor" {
			actors = append(actors, pr.Name+"="+pr.Role)
		} else {
			crew = append(crew, pr.Kind+":"+pr.Name)
		}
	}
	if strings.Join(actors, ",") != "Audrey Tautou=Amélie Poulain,Mathieu Kassovitz=Nino Quincampoix,Lorella Cravotta=Amandine Fouet,Rufus=Raphaël Poulain" {
		t.Fatalf("actors = %v", actors)
	}
	if strings.Join(crew, ",") != "director:Jean-Pierre Jeunet,writer:Jean-Pierre Jeunet,writer:Guillaume Laurant,composer:Yann Tiersen,producer:Claudie Ossard" {
		t.Fatalf("crew = %v", crew)
	}
	a := r.People[0]
	if a.Order != 0 || a.ImageURL != imgBase+"w185/4F7OGwZlqFAtLBq7ySAJsWufd8S.jpg" || a.ProviderIDs["tmdb"] != "1013" {
		t.Fatalf("actor = %+v", a)
	}
	if r.People[3].ImageURL != "" {
		t.Fatalf("no profile path should give no image: %+v", r.People[3])
	}
	// Images.
	var got []string
	for _, im := range r.Images {
		got = append(got, im.Type+":"+strings.TrimPrefix(im.URL, imgBase+"original")+":"+im.Language)
	}
	want := []string{
		"poster:/vZ9NhNbQQ3yhtiC5sbhpy5KTXns.jpg:fr", "poster:/frPosterB.jpg:fr", "poster:/enPoster.jpg:en", "poster:/nullPoster.jpg:",
		"backdrop:/x7ajSAWoS2y9vSMNHVQvvVeKaxq.jpg:", "backdrop:/frBackdrop.jpg:fr",
		"logo:/frLogo.png:fr", "logo:/enLogo.png:en",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("images =\n%v\nwant\n%v", got, want)
	}
	if im := r.Images[0]; im.Width != 2000 || im.Height != 3000 || im.Rating != 5.6 {
		t.Fatalf("image dims = %+v", im)
	}
	// Overview and tagline present: no en-US fallback request.
	if n := len(f.requests("/3/movie/194")); n != 1 {
		t.Fatalf("requests = %d", n)
	}
}

func TestFetchMovieCollectionAndEnglishFallback(t *testing.T) {
	p, _ := newProvider(t, testKey, "")
	r, err := p.Fetch(context.Background(), metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"tmdb": "24"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Collection != "Kill Bill - Saga" || r.ProviderIDs["tmdbcollection"] != "2883" {
		t.Fatalf("collection: %q %v", r.Collection, r.ProviderIDs)
	}
	if !strings.HasPrefix(r.Overview, "An assassin") || r.Tagline != "Go for the kill." || r.Title != "Kill Bill : Volume 1" {
		t.Fatalf("fallback: %+v", r)
	}
	if r.OfficialRating != "R" {
		t.Fatalf("US fallback certification = %q", r.OfficialRating)
	}
	// No image lists: fall back to poster_path / backdrop_path.
	if len(r.Images) != 2 || r.Images[0].URL != imgBase+"original/v7TaX8kXMXs5yFFGR41guUDNcnB.jpg" || r.Images[1].Type != metadata.ImageBackdrop {
		t.Fatalf("images = %+v", r.Images)
	}
}

func TestFetchMovieByImdbOnly(t *testing.T) {
	p, f := newProvider(t, testKey, "")
	r, err := p.Fetch(context.Background(), metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"imdb": "tt0211915"}})
	if err != nil || r.ProviderIDs["tmdb"] != "194" {
		t.Fatalf("got %v, %v", r, err)
	}
	if len(f.requests("/3/find/")) != 1 {
		t.Fatal("expected a /find request")
	}
}

func TestFetchNotFound(t *testing.T) {
	p, _ := newProvider(t, testKey, "")
	ctx := context.Background()
	if _, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"tmdb": "999"}}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	if _, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindMovie}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("no id: %v", err)
	}
	if _, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"imdb": "tt0000000"}}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unknown imdb: %v", err)
	}
}

func TestCacheAvoidsSecondRequest(t *testing.T) {
	p, f := newProvider(t, testKey, t.TempDir())
	q := metadata.Query{Kind: metadata.KindMovie, ProviderIDs: metadata.ProviderIDs{"tmdb": "194"}}
	for i := 0; i < 2; i++ {
		if _, err := p.Fetch(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.requests("/3/movie/194")); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
	// Search by tmdb id reuses the cached details as well.
	if _, err := p.Search(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if n := len(f.requests("/3/movie/194")); n != 1 {
		t.Fatalf("requests = %d after search, want 1", n)
	}
}

func TestSearchSeries(t *testing.T) {
	p, f := newProvider(t, testKey, "")
	cs, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindSeries, Title: "Breaking Bad", Year: 2008})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].ProviderIDs["tmdb"] != "1396" || cs[0].Year != 2008 || cs[0].Score < 0.95 {
		t.Fatalf("got %+v", cs)
	}
	if q := f.requests("/3/search/tv")[0].URL.Query(); q.Get("first_air_date_year") != "2008" {
		t.Fatalf("params = %v", q)
	}
}

func TestFetchSeries(t *testing.T) {
	p, _ := newProvider(t, testKey, "")
	r, err := p.Fetch(context.Background(), metadata.Query{Kind: metadata.KindSeries, ProviderIDs: metadata.ProviderIDs{"tvdb": "81189"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "Breaking Bad" || r.Tagline != "Le roi de la chimie." || r.PremiereDate != "2008-01-20" || r.Year != 2008 {
		t.Fatalf("basics: %+v", r)
	}
	if r.Status != "ended" || r.EndDate != "2013-09-29" || r.RuntimeMs != 45*60_000 || r.OfficialRating != "16" {
		t.Fatalf("status/end/runtime/rating: %+v", r)
	}
	if r.ProviderIDs["tmdb"] != "1396" || r.ProviderIDs["tvdb"] != "81189" || r.ProviderIDs["imdb"] != "tt0903747" {
		t.Fatalf("ids: %v", r.ProviderIDs)
	}
	if strings.Join(r.Studios, "|") != "AMC|Sony Pictures Television Studios|High Bridge Productions|Gran Via Productions" {
		t.Fatalf("studios: %v", r.Studios)
	}
	var people []string
	for _, pr := range r.People {
		people = append(people, pr.Kind+":"+pr.Name+"="+pr.Role)
	}
	want := "actor:Bryan Cranston=Walter White,actor:Aaron Paul=Jesse Pinkman,actor:Steven Michael Quezada=Steven Gomez,writer:Vince Gilligan=Creator"
	if strings.Join(people, ",") != want {
		t.Fatalf("people = %v", people)
	}
	if len(r.Images) != 4 || r.Images[0].Language != "fr" || r.Images[2].Type != metadata.ImageBackdrop || r.Images[3].Type != metadata.ImageLogo {
		t.Fatalf("images = %+v", r.Images)
	}
}

func TestSeriesStatus(t *testing.T) {
	for in, want := range map[string]string{"Returning Series": "continuing", "In Production": "continuing", "Ended": "ended", "Canceled": "ended", "Planned": ""} {
		if got := seriesStatus(in); got != want {
			t.Errorf("%s → %q", in, got)
		}
	}
}

func TestFetchSeasonAndEpisodes(t *testing.T) {
	p, f := newProvider(t, testKey, "")
	ctx := context.Background()
	parent := metadata.ProviderIDs{"tmdb": "1396"}
	s, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindSeason, ParentIDs: parent, IndexNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Saison 1" || s.IndexNumber != 1 || s.PremiereDate != "2008-01-20" || !strings.HasPrefix(s.Overview, "Professeur") {
		t.Fatalf("season: %+v", s)
	}
	if len(s.Images) != 2 || s.Images[0].Language != "fr" || s.Images[0].Type != metadata.ImagePoster {
		t.Fatalf("season images: %+v", s.Images)
	}
	if len(s.Children) != 2 {
		t.Fatalf("children: %v", s.Children)
	}
	e1 := s.Children[1]
	if e1.Title != "Chute libre" || e1.IndexNumber != 1 || e1.ParentIndex != 1 || e1.RuntimeMs != 59*60_000 ||
		e1.CommunityRating != 8.3 || e1.PremiereDate != "2008-01-20" || e1.ProviderIDs["tmdb"] != "62085" {
		t.Fatalf("e1: %+v", e1)
	}
	if len(e1.Images) != 1 || e1.Images[0].Type != metadata.ImageThumb || e1.Images[0].URL != imgBase+"original/ydlY3iPfeOAvu8gVqrxPoMvzNCn.jpg" {
		t.Fatalf("e1 images: %+v", e1.Images)
	}
	var people []string
	for _, pr := range e1.People {
		people = append(people, pr.Kind+":"+pr.Name)
	}
	if strings.Join(people, ",") != "director:Vince Gilligan,writer:Vince Gilligan,guest:John Koyama,guest:Marius Stan" {
		t.Fatalf("e1 people: %v", people)
	}
	// French overview missing → English one.
	if e2 := s.Children[2]; e2.Title != "Le Choix" || e2.Overview != "Walt and Jesse attempt to tie up loose ends." {
		t.Fatalf("e2: %+v", e2)
	}

	// Episodes come from the same season fetch: no new request.
	for _, n := range []int{1, 2} {
		e, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindEpisode, ParentIDs: parent, ParentIndex: 1, IndexNumber: n})
		if err != nil || e.IndexNumber != n {
			t.Fatalf("episode %d: %+v %v", n, e, err)
		}
	}
	if n := len(f.requests("/3/tv/1396/season/1")); n != 2 { // fr + en-US overview fallback
		t.Fatalf("season requests = %d, want 2", n)
	}
	if _, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindEpisode, ParentIDs: parent, ParentIndex: 1, IndexNumber: 9}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("missing episode: %v", err)
	}
	if _, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindSeason, ParentIDs: parent, IndexNumber: 7}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("missing season: %v", err)
	}

	cs, err := p.Search(ctx, metadata.Query{Kind: metadata.KindEpisode, ParentIDs: parent, ParentIndex: 1, IndexNumber: 2})
	if err != nil || len(cs) != 1 || cs[0].Score != 1 || cs[0].Title != "Le Choix" {
		t.Fatalf("episode search: %+v %v", cs, err)
	}
}

func TestImageLanguagesConfig(t *testing.T) {
	p := New(Config{APIKey: "k", ImageLanguages: []string{"de", "null", "en", "de"}}, nil)
	if got := p.includeImageLanguage(metadata.Query{}); got != "de,null,en" {
		t.Fatalf("got %q", got)
	}
	p = New(Config{APIKey: "k"}, nil)
	if got := p.includeImageLanguage(metadata.Query{Language: "en-US"}); got != "en,null" {
		t.Fatalf("got %q", got)
	}
	if got := countries(metadata.Query{Language: "de-DE"}, p); strings.Join(got, ",") != "DE,US" {
		t.Fatalf("countries %v", got)
	}
}
