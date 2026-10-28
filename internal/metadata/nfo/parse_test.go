package nfo

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

func parseFixture(t *testing.T, name, kind string) *metadata.Result {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := ParseNFO(f, kind)
	if err != nil {
		t.Fatalf("ParseNFO(%s): %v", name, err)
	}
	return r
}

func eq(t *testing.T, field string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", field, got, want)
	}
}

func TestParseFixtures(t *testing.T) {
	tests := []struct {
		file, kind string
		check      func(t *testing.T, r *metadata.Result)
	}{
		{"movie_kodi.nfo", metadata.KindMovie, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Le Fabuleux Destin d'Amélie Poulain")
			eq(t, "OriginalTitle", r.OriginalTitle, "Le Fabuleux Destin d'Amélie Poulain")
			eq(t, "SortTitle", r.SortTitle, "Fabuleux Destin d'Amélie Poulain")
			eq(t, "Overview", r.Overview, "Amélie, une jeune serveuse dans un bar de Montmartre, passe son temps à observer les gens.")
			eq(t, "Tagline", r.Tagline, "Elle va changer votre vie.")
			eq(t, "Year", r.Year, 2001)
			eq(t, "PremiereDate", r.PremiereDate, "2001-04-25")
			eq(t, "RuntimeMs", r.RuntimeMs, int64(122*60_000))
			eq(t, "OfficialRating", r.OfficialRating, "U")
			eq(t, "CommunityRating", r.CommunityRating, 7.9)
			eq(t, "Genres", r.Genres, []string{"Comédie", "Romance"})
			eq(t, "Studios", r.Studios, []string{"Claudie Ossard Productions", "UGC"})
			eq(t, "Collection", r.Collection, "Jean-Pierre Jeunet Collection")
			eq(t, "ProviderIDs", r.ProviderIDs, metadata.ProviderIDs{"tmdb": "194", "imdb": "tt0211915"})
			if len(r.People) != 5 {
				t.Fatalf("People = %+v", r.People)
			}
			eq(t, "People[0]", r.People[0], metadata.Person{Name: "Audrey Tautou", Role: "Amélie Poulain", Kind: "actor",
				ImageURL: "https://image.tmdb.org/t/p/h632/tautou.jpg", ProviderIDs: metadata.ProviderIDs{"tmdb": "2405"}})
			eq(t, "People[1].Order", r.People[1].Order, 1)
			eq(t, "People[2]", r.People[2].Name+"/"+r.People[2].Kind, "Jean-Pierre Jeunet/director")
			eq(t, "People[3]", r.People[3].Name+"/"+r.People[3].Kind, "Guillaume Laurant/writer")
			eq(t, "Images", r.Images, []metadata.RemoteImage{
				{Type: "poster", URL: "https://image.tmdb.org/t/p/original/poster.jpg"},
				{Type: "logo", URL: "https://assets.fanart.tv/fanart/movies/194/hdmovielogo/logo.png"},
				{Type: "backdrop", URL: "https://image.tmdb.org/t/p/original/fanart.jpg"},
			})
		}},
		{"movie_jellyfin.nfo", metadata.KindMovie, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Inception")
			eq(t, "Overview", r.Overview, "A thief who steals corporate secrets through the use of dream-sharing technology.")
			eq(t, "OfficialRating", r.OfficialRating, "PG-13")
			eq(t, "CommunityRating", r.CommunityRating, 8.4)
			eq(t, "Genres", r.Genres, []string{"Action", "Science Fiction"})
			eq(t, "ProviderIDs", r.ProviderIDs, metadata.ProviderIDs{"imdb": "tt1375666", "tmdb": "27205"})
			eq(t, "Collection", r.Collection, "Inception Collection")
			eq(t, "RuntimeMs", r.RuntimeMs, int64(148*60_000))
			// writer + credits with the same name collapse to one writer.
			eq(t, "len(People)", len(r.People), 3)
		}},
		{"tvshow.nfo", metadata.KindSeries, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Breaking Bad")
			eq(t, "Status", r.Status, "ended")
			eq(t, "PremiereDate", r.PremiereDate, "2008-01-20")
			eq(t, "EndDate", r.EndDate, "2013-09-29")
			eq(t, "Year", r.Year, 2008)
			eq(t, "OfficialRating", r.OfficialRating, "TV-MA")
			eq(t, "CommunityRating", r.CommunityRating, 8.9)
			eq(t, "ProviderIDs", r.ProviderIDs, metadata.ProviderIDs{"tvdb": "81189", "imdb": "tt0903747", "tmdb": "1396"})
			eq(t, "Children[1].Title", r.Children[1].Title, "Season One")
			eq(t, "Children[0].Title", r.Children[0].Title, "Specials")
			eq(t, "Images", r.Images, []metadata.RemoteImage{
				{Type: "poster", URL: "https://artworks.thetvdb.com/banners/posters/81189-1.jpg"},
				{Type: "banner", URL: "https://artworks.thetvdb.com/banners/graphical/81189-g.jpg"},
			})
		}},
		{"episode_multi.nfo", metadata.KindEpisode, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Pilot (1)")
			eq(t, "ParentIndex", r.ParentIndex, 1)
			eq(t, "IndexNumber", r.IndexNumber, 1)
			eq(t, "PremiereDate", r.PremiereDate, "2004-09-22")
			eq(t, "ProviderIDs", r.ProviderIDs, metadata.ProviderIDs{"tvdb": "127131"})
			eq(t, "People[0].Kind", r.People[0].Kind, "guest")
			if len(r.Children) != 1 || r.Children[2] == nil {
				t.Fatalf("Children = %+v", r.Children)
			}
			c := r.Children[2]
			eq(t, "child.Title", c.Title, "Pilot (2)")
			eq(t, "child.IndexNumber", c.IndexNumber, 2)
			eq(t, "child.ProviderIDs", c.ProviderIDs, metadata.ProviderIDs{"tvdb": "127132"})
		}},
		{"season.nfo", metadata.KindSeason, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Season 1")
			eq(t, "IndexNumber", r.IndexNumber, 1)
			eq(t, "Overview", r.Overview, "The first season.")
			eq(t, "ProviderIDs", r.ProviderIDs, metadata.ProviderIDs{"tvdb": "30272"})
		}},
		{"album.nfo", metadata.KindAlbum, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "OK Computer")
			eq(t, "Overview", r.Overview, "Radiohead's third album.")
			eq(t, "Year", r.Year, 1997)
			eq(t, "PremiereDate", r.PremiereDate, "1997-05-21")
			eq(t, "CommunityRating", r.CommunityRating, 8.7)
			eq(t, "Genres", r.Genres, []string{"Alternative Rock", "Art Rock"})
			eq(t, "Studios", r.Studios, []string{"Parlophone"})
			eq(t, "ProviderIDs", r.ProviderIDs, metadata.ProviderIDs{
				"musicbrainzrelease":      "0b6b4ba0-d36f-47bd-b4ea-6a5b91842d29",
				"musicbrainzreleasegroup": "b1392450-e666-3926-a536-22c65f834433",
			})
			eq(t, "People", r.People, []metadata.Person{{Name: "Radiohead", Kind: "artist",
				ProviderIDs: metadata.ProviderIDs{"musicbrainzartist": "a74b1b7f-71a5-4011-9441-d0b5e4122711"}}})
			eq(t, "Children[1]", *r.Children[1], metadata.Result{Title: "Airbag", IndexNumber: 1, RuntimeMs: 284_000})
			eq(t, "Children[2].ids", r.Children[2].ProviderIDs, metadata.ProviderIDs{"musicbrainztrack": "e5d4a9c2-0a2f-4b7c-8e0d-6b3f1a2c4d5e"})
		}},
		{"artist.nfo", metadata.KindArtist, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Daft Punk")
			eq(t, "SortTitle", r.SortTitle, "Daft Punk")
			eq(t, "Overview", r.Overview, "Daft Punk were a French electronic music duo formed in 1993 in Paris.")
			eq(t, "Year", r.Year, 1993)
			eq(t, "EndDate", r.EndDate, "2021-02-22")
			eq(t, "Genres", r.Genres, []string{"Electronic", "House"})
			eq(t, "ProviderIDs", r.ProviderIDs, metadata.ProviderIDs{"musicbrainzartist": "056e4f3e-d505-4dad-8ec1-d04f521cbb56"})
			eq(t, "Images", r.Images, []metadata.RemoteImage{{Type: "poster", URL: "https://assets.fanart.tv/fanart/music/daftpunk.jpg"}})
		}},
		{"url_imdb.nfo", metadata.KindMovie, func(t *testing.T, r *metadata.Result) {
			eq(t, "result", *r, metadata.Result{ProviderIDs: metadata.ProviderIDs{"imdb": "tt0133093"}})
		}},
		{"url_tmdb.nfo", metadata.KindMovie, func(t *testing.T, r *metadata.Result) {
			eq(t, "result", *r, metadata.Result{ProviderIDs: metadata.ProviderIDs{"tmdb": "603"}})
		}},
		{"url_tvdb.nfo", metadata.KindSeries, func(t *testing.T, r *metadata.Result) {
			eq(t, "result", *r, metadata.Result{ProviderIDs: metadata.ProviderIDs{"tvdb": "81189"}})
		}},
		{"xml_then_url.nfo", metadata.KindMovie, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "The Matrix")
			eq(t, "ProviderIDs", r.ProviderIDs, metadata.ProviderIDs{"tmdb": "603"})
		}},
		{"broken.nfo", metadata.KindMovie, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Broken été &unknown; Movie & Co")
			if !strings.HasPrefix(r.Overview, "Unclosed") {
				t.Errorf("Overview = %q", r.Overview)
			}
		}},
		{"bom_utf8_declared_latin1.nfo", metadata.KindMovie, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Amélie")
		}},
		{"latin1.nfo", metadata.KindMovie, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "La Cité des enfants perdus")
			eq(t, "Overview", r.Overview, "“Quoted” € café")
			eq(t, "Year", r.Year, 1995)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) { tt.check(t, parseFixture(t, tt.file, tt.kind)) })
	}
}

func TestParseEmpty(t *testing.T) {
	for _, s := range []string{"", "not an nfo at all", "<movie></movie>", "<<<>>>"} {
		if r, err := ParseNFO(strings.NewReader(s), metadata.KindMovie); !errors.Is(err, ErrEmpty) {
			t.Errorf("ParseNFO(%q) = %+v, %v; want ErrEmpty", s, r, err)
		}
	}
}

func TestParseUTF16(t *testing.T) {
	s := `<?xml version="1.0" encoding="UTF-16"?><movie><title>Léon</title></movie>`
	b := []byte{0xFF, 0xFE}
	for _, r := range s {
		b = append(b, byte(r), byte(r>>8))
	}
	r, err := ParseNFO(strings.NewReader(string(b)), metadata.KindMovie)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "Title", r.Title, "Léon")
}

func TestNormCertification(t *testing.T) {
	for in, want := range map[string]string{
		"Rated R":                    "R",
		"PG-13":                      "PG-13",
		"US:PG-13 / FR:12":           "12",
		"US:Rated PG":                "PG",
		"DE:16 / GB:15":              "16",
		"DE:16 / US:R":               "R",
		"France:-12":                 "-12",
		"TV-14":                      "TV-14",
		"":                           "",
		"Germany:FSK 12 | USA:PG-13": "PG-13",
	} {
		if got := normCertification(in); got != want {
			t.Errorf("normCertification(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSnippets(t *testing.T) {
	tests := []struct {
		name, kind, in string
		check          func(t *testing.T, r *metadata.Result)
	}{
		{"legacy id movie numeric", metadata.KindMovie, `<movie><title>X</title><id>603</id></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "ids", r.ProviderIDs, metadata.ProviderIDs{"tmdb": "603"})
		}},
		{"legacy id series", metadata.KindSeries, `<tvshow><title>X</title><id>81189</id><imdb_id>0903747</imdb_id></tvshow>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "ids", r.ProviderIDs, metadata.ProviderIDs{"tvdb": "81189", "imdb": "tt0903747"})
		}},
		{"uniqueid wins over id", metadata.KindMovie, `<movie><id>tt1</id><uniqueid type="imdb">tt0133093</uniqueid></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "ids", r.ProviderIDs, metadata.ProviderIDs{"imdb": "tt0133093"})
		}},
		{"outline fallback", metadata.KindMovie, `<movie><plot></plot><outline>Out.</outline></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "Overview", r.Overview, "Out.")
		}},
		{"rating 0-100 max", metadata.KindMovie, `<movie><title>X</title><ratings><rating name="mc" max="100"><value>85</value></rating></ratings></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "CommunityRating", r.CommunityRating, 8.5)
		}},
		{"rating comma", metadata.KindMovie, `<movie><title>X</title><rating>7,5</rating></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "CommunityRating", r.CommunityRating, 7.5)
		}},
		{"set text with tmdb id", metadata.KindMovie, `<movie><set tmdbcolid="2344">The Matrix Collection</set></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "Collection", r.Collection, "The Matrix Collection")
			eq(t, "ids", r.ProviderIDs, metadata.ProviderIDs{"tmdbcollection": "2344"})
		}},
		{"runtime with unit", metadata.KindMovie, `<movie><title>X</title><runtime>95 min</runtime><aired>1999-3-1</aired></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "RuntimeMs", r.RuntimeMs, int64(95*60_000))
			eq(t, "PremiereDate", r.PremiereDate, "1999-03-01")
			eq(t, "Year", r.Year, 1999)
		}},
		{"runtime from streamdetails", metadata.KindEpisode, `<episodedetails><title>X</title><fileinfo><streamdetails><video><durationinseconds>2520</durationinseconds></video></streamdetails></fileinfo></episodedetails>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "RuntimeMs", r.RuntimeMs, int64(2_520_000))
		}},
		{"status continuing", metadata.KindSeries, `<tvshow><title>X</title><status>Returning Series</status></tvshow>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "Status", r.Status, "continuing")
		}},
		{"kodi20 episodeguide url", metadata.KindSeries, `<tvshow><title>X</title><episodeguide><url cache="81189-en.xml">http://thetvdb.com/api/1D62F2F90030C444/series/81189/all/en.zip</url></episodeguide></tvshow>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "ids", r.ProviderIDs, metadata.ProviderIDs{"tvdb": "81189"})
		}},
		{"episode not multi without kind", metadata.KindMovie, `<movie><title>A</title></movie><movie><title>B</title></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "A")
			eq(t, "Children", len(r.Children), 0)
		}},
		{"album artist plain", metadata.KindAlbum, `<album><title>Discovery</title><artist>Daft Punk</artist><musicbrainzalbumartistid>056e4f3e-d505-4dad-8ec1-d04f521cbb56</musicbrainzalbumartistid></album>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "People", r.People, []metadata.Person{{Name: "Daft Punk", Kind: "artist",
				ProviderIDs: metadata.ProviderIDs{"musicbrainzartist": "056e4f3e-d505-4dad-8ec1-d04f521cbb56"}}})
		}},
		{"invalid date and year", metadata.KindMovie, `<movie><title>X</title><year>0</year><premiered>2010-13-45</premiered></movie>`, func(t *testing.T, r *metadata.Result) {
			eq(t, "PremiereDate", r.PremiereDate, "")
			eq(t, "Year", r.Year, 0)
		}},
		{"truncated file keeps data", metadata.KindMovie, `<movie><title>Cut</title><plot>abc`, func(t *testing.T, r *metadata.Result) {
			eq(t, "Title", r.Title, "Cut")
			eq(t, "Overview", r.Overview, "abc")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseNFO(strings.NewReader(tt.in), tt.kind)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, r)
		})
	}
}
