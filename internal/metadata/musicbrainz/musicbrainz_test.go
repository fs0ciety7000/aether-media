package musicbrainz

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
	daftPunk  = "056e4f3e-d505-4dad-8ec1-d04f521cbb56"
	discovery = "48117b90-a16e-34ca-a514-19c702df1158"
	release1  = "bd3bb36e-16c8-438f-850e-dfbf4d1478f0"
	testUA    = "Aether/test ( https://github.com/fs0ciety7000/aether-media )"
	caa       = "https://caa.test"
)

type fakeMB struct {
	t    *testing.T
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []*http.Request
}

func newFake(t *testing.T) *fakeMB {
	f := &fakeMB{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeMB) last() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reqs[len(f.reqs)-1]
}

func (f *fakeMB) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r.Clone(context.Background()))
	f.mu.Unlock()
	if r.URL.Query().Get("fmt") != "json" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	q := r.URL.Query().Get("query")
	file := ""
	switch r.URL.Path {
	case "/ws/2/artist":
		if strings.Contains(q, "Daft Punk") {
			file = "search_artist_daft_punk.json"
		}
	case "/ws/2/release-group":
		if strings.Contains(q, "Discovery") {
			file = "search_release_group_discovery.json"
		}
	case "/ws/2/artist/" + daftPunk:
		file = "artist_daft_punk.json"
	case "/ws/2/release-group/" + discovery:
		file = "release_group_discovery.json"
	case "/ws/2/release/" + release1:
		w.Write([]byte(`{"id":"` + release1 + `","title":"Discovery","release-group":{"id":"` + discovery + `","title":"Discovery"}}`))
		return
	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Not Found","help":"For usage, please see: https://musicbrainz.org/development/mmd"}`))
		return
	}
	if file == "" {
		w.Write([]byte(`{"count":0,"offset":0,"artists":[],"release-groups":[]}`))
		return
	}
	b, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		f.t.Errorf("fixture: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

func newProvider(t *testing.T) (*Provider, *fakeMB) {
	f := newFake(t)
	c := httpx.New(httpx.Options{UserAgent: testUA})
	return New(Config{BaseURL: f.srv.URL + "/ws/2", CoverArtURL: caa}, c), f
}

func TestSupports(t *testing.T) {
	p, _ := newProvider(t)
	if p.Name() != "musicbrainz" || !p.Supports(metadata.KindArtist) || !p.Supports(metadata.KindAlbum) || p.Supports(metadata.KindMovie) {
		t.Fatal("supports")
	}
}

func TestSearchArtist(t *testing.T) {
	p, f := newProvider(t)
	cs, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindArtist, Title: "Daft Punk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 || cs[0].ProviderIDs[metadata.IDMusicBrainzArtist] != daftPunk || cs[0].Score != 1 {
		t.Fatalf("got %+v", cs)
	}
	if cs[0].Overview != "Group, FR — French electronic duo" || cs[0].Year != 1993 || cs[0].Provider != "musicbrainz" {
		t.Fatalf("candidate = %+v", cs[0])
	}
	if !(cs[1].Score < cs[0].Score && cs[2].Score <= cs[1].Score) {
		t.Fatalf("not sorted: %+v", cs)
	}
	r := f.last()
	if got := r.URL.Query().Get("query"); got != `artist:"Daft Punk"` {
		t.Fatalf("query = %q", got)
	}
	if r.URL.Query().Get("limit") != "10" || r.Header.Get("User-Agent") != testUA {
		t.Fatalf("limit/UA: %v %q", r.URL.Query(), r.Header.Get("User-Agent"))
	}
}

func TestLuceneEscape(t *testing.T) {
	p, f := newProvider(t)
	if _, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindArtist, Title: `Florence + the "Machine" \o/`}); err != nil {
		t.Fatal(err)
	}
	if got := f.last().URL.Query().Get("query"); got != `artist:"Florence + the \"Machine\" \\o/"` {
		t.Fatalf("query = %q", got)
	}
}

func TestSearchAlbum(t *testing.T) {
	p, f := newProvider(t)
	cs, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindAlbum, Title: "Discovery", ParentTitle: "Daft Punk", Year: 2001})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.last().URL.Query().Get("query"); got != `releasegroup:"Discovery" AND artist:"Daft Punk"` {
		t.Fatalf("query = %q", got)
	}
	if len(cs) != 8 {
		t.Fatalf("len = %d", len(cs))
	}
	c := cs[0]
	if c.ProviderIDs[metadata.IDMusicBrainzRelGrp] != discovery || c.Score != 1 || c.Year != 2001 ||
		c.ImageURL != caa+"/release-group/"+discovery+"/front-250" || c.Overview != "Daft Punk — Album" {
		t.Fatalf("candidate = %+v", c)
	}
	// "Discovery (Beta Version)" (2001) shares the year but not the title.
	for _, x := range cs[1:] {
		if x.Score >= c.Score {
			t.Fatalf("%q scored %v ≥ exact match", x.Title, x.Score)
		}
	}

	// No album artist: no artist clause.
	if _, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindAlbum, Title: "Discovery"}); err != nil {
		t.Fatal(err)
	}
	if got := f.last().URL.Query().Get("query"); got != `releasegroup:"Discovery"` {
		t.Fatalf("query = %q", got)
	}
}

func TestSearchByID(t *testing.T) {
	p, f := newProvider(t)
	cs, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindArtist, Title: "x",
		ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzArtist: daftPunk}})
	if err != nil || len(cs) != 1 || cs[0].Score != 1 || cs[0].Title != "Daft Punk" {
		t.Fatalf("got %+v %v", cs, err)
	}
	if f.last().URL.Path != "/ws/2/artist/"+daftPunk {
		t.Fatalf("path = %s", f.last().URL.Path)
	}
}

func TestFetchArtist(t *testing.T) {
	p, f := newProvider(t)
	r, err := p.Fetch(context.Background(), metadata.Query{Kind: metadata.KindArtist,
		ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzArtist: daftPunk}})
	if err != nil {
		t.Fatal(err)
	}
	if raw := f.last().URL.RawQuery; !strings.Contains(raw, "inc=genres+tags+url-rels") {
		t.Fatalf("raw query = %s", raw)
	}
	if r.Title != "Daft Punk" || r.SortTitle != "Daft Punk" || r.Year != 1993 || r.PremiereDate != "" || r.EndDate != "2021-02-22" {
		t.Fatalf("basics: %+v", r)
	}
	if r.Overview != "" || len(r.Images) != 0 {
		t.Fatalf("overview/images should stay empty: %+v", r)
	}
	if len(r.Genres) != maxGenres || strings.Join(r.Genres[:3], "|") != "Electronic|House|French House" {
		t.Fatalf("genres = %v", r.Genres)
	}
	if r.ProviderIDs[metadata.IDMusicBrainzArtist] != daftPunk {
		t.Fatalf("ids = %v", r.ProviderIDs)
	}
}

func TestFetchAlbum(t *testing.T) {
	p, f := newProvider(t)
	r, err := p.Fetch(context.Background(), metadata.Query{Kind: metadata.KindAlbum,
		ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzRelGrp: discovery}})
	if err != nil {
		t.Fatal(err)
	}
	if raw := f.last().URL.RawQuery; !strings.Contains(raw, "inc=genres+artist-credits+releases") {
		t.Fatalf("raw query = %s", raw)
	}
	if r.Title != "Discovery" || r.Year != 2001 || r.PremiereDate != "2001-02-26" {
		t.Fatalf("basics: %+v", r)
	}
	if strings.Join(r.Genres[:3], "|") != "House|Electronic|French House" {
		t.Fatalf("genres = %v", r.Genres)
	}
	if r.ProviderIDs[metadata.IDMusicBrainzRelGrp] != discovery || r.ProviderIDs[metadata.IDMusicBrainzRelease] != release1 {
		t.Fatalf("ids = %v", r.ProviderIDs)
	}
	if len(r.People) != 1 || r.People[0].Name != "Daft Punk" || r.People[0].Kind != "artist" ||
		r.People[0].ProviderIDs[metadata.IDMusicBrainzArtist] != daftPunk {
		t.Fatalf("people = %+v", r.People)
	}
	if len(r.Images) != 1 || r.Images[0].Type != metadata.ImagePoster || r.Images[0].URL != caa+"/release-group/"+discovery+"/front-500" {
		t.Fatalf("images = %+v", r.Images)
	}
}

func TestFetchAlbumByReleaseID(t *testing.T) {
	p, _ := newProvider(t)
	r, err := p.Fetch(context.Background(), metadata.Query{Kind: metadata.KindAlbum,
		ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzRelease: release1}})
	if err != nil || r.ProviderIDs[metadata.IDMusicBrainzRelGrp] != discovery {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestFetchNotFound(t *testing.T) {
	p, _ := newProvider(t)
	ctx := context.Background()
	for _, q := range []metadata.Query{
		{Kind: metadata.KindArtist},
		{Kind: metadata.KindAlbum},
		{Kind: metadata.KindArtist, ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzArtist: "00000000-0000-0000-0000-000000000000"}},
		{Kind: metadata.KindAlbum, ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzRelGrp: "00000000-0000-0000-0000-000000000000"}},
	} {
		if _, err := p.Fetch(ctx, q); !errors.Is(err, metadata.ErrNotFound) {
			t.Errorf("%+v: %v", q, err)
		}
	}
}

func TestBusyServerIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"The MusicBrainz web server is currently busy. Please try again later."}`))
	}))
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL}, httpx.New(httpx.Options{UserAgent: testUA}))
	_, err := p.Search(context.Background(), metadata.Query{Kind: metadata.KindArtist, Title: "Daft Punk"})
	if !errors.Is(err, metadata.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestTopGenres(t *testing.T) {
	if got := topGenres(nil, []counted{{"rock", 0}, {"hip hop", 3}, {"r&b", 5}}); strings.Join(got, "|") != "R&B|Hip Hop" {
		t.Fatalf("tags fallback = %v", got)
	}
	if got := firstRelease([]release{{ID: "a", Date: "2002", Status: "Official"}, {ID: "b", Date: "", Status: "Official"},
		{ID: "c", Date: "1999", Status: "Bootleg"}, {ID: "d", Date: "2001-05-01", Status: "Official"}}); got != "d" {
		t.Fatalf("firstRelease = %s", got)
	}
}

// TestLive hits the real MusicBrainz service (AETHER_LIVE_TESTS=1).
func TestLive(t *testing.T) {
	if os.Getenv("AETHER_LIVE_TESTS") != "1" {
		t.Skip("set AETHER_LIVE_TESTS=1 to query musicbrainz.org")
	}
	c := httpx.New(httpx.Options{UserAgent: testUA, RatePerSec: 1})
	p := New(Config{}, c)
	ctx := context.Background()
	cs, err := p.Search(ctx, metadata.Query{Kind: metadata.KindArtist, Title: "Daft Punk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) == 0 || cs[0].ProviderIDs[metadata.IDMusicBrainzArtist] != daftPunk {
		t.Fatalf("artist search: %+v", cs)
	}
	r, err := p.Fetch(ctx, metadata.Query{Kind: metadata.KindAlbum, ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzRelGrp: discovery}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "Discovery" || r.Year != 2001 {
		t.Fatalf("album: %+v", r)
	}
}
