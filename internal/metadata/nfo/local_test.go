package nfo

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

// mkTree creates files (relative paths, "/"-separated) under a temp dir.
// Values are file contents; "" writes a placeholder.
func mkTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, "/") {
			continue
		}
		if content == "" {
			content = "x"
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// imgs returns "type:relative/path" for each image, in order.
func imgs(root string, list []metadata.RemoteImage) []string {
	out := []string{}
	for _, im := range list {
		rel, _ := filepath.Rel(root, im.URL)
		out = append(out, im.Type+":"+filepath.ToSlash(rel))
	}
	return out
}

func rel(root, p string) string {
	if p == "" {
		return ""
	}
	r, _ := filepath.Rel(root, p)
	return filepath.ToSlash(r)
}

func TestFindNFO(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		q     metadata.Query // Path relative to root
		want  string
	}{
		{"movie.nfo in dedicated folder", []string{"Heat (1995)/Heat (1995).mkv", "Heat (1995)/movie.nfo", "Heat (1995)/Heat (1995).nfo"},
			metadata.Query{Kind: "movie", Path: "Heat (1995)", Title: "Heat"}, "Heat (1995)/movie.nfo"},
		{"basename nfo, case-insensitive", []string{"Heat (1995)/Heat (1995).mkv", "Heat (1995)/HEAT (1995).NFO"},
			metadata.Query{Kind: "movie", Path: "Heat (1995)", Title: "Heat"}, "Heat (1995)/HEAT (1995).NFO"},
		{"stacked movie nfo without part", []string{"Heat/Heat (1995) cd1.avi", "Heat/Heat (1995) cd2.avi", "Heat/Heat (1995).nfo"},
			metadata.Query{Kind: "movie", Path: "Heat", Title: "Heat"}, "Heat/Heat (1995).nfo"},
		{"stacked movie nfo of part 1", []string{"Heat/Heat (1995) cd1.avi", "Heat/Heat (1995) cd2.avi", "Heat/Heat (1995) cd1.nfo"},
			metadata.Query{Kind: "movie", Path: "Heat", Title: "Heat"}, "Heat/Heat (1995) cd1.nfo"},
		{"flat folder picks by title", []string{"Movies/Heat (1995).mkv", "Movies/Heat (1995).nfo", "Movies/Alien (1979).mkv", "Movies/Alien (1979).nfo", "Movies/movie.nfo"},
			metadata.Query{Kind: "movie", Path: "Movies", Title: "Alien", Year: 1979}, "Movies/Alien (1979).nfo"},
		{"flat folder never uses movie.nfo", []string{"Movies/Heat (1995).mkv", "Movies/Alien (1979).mkv", "Movies/movie.nfo"},
			metadata.Query{Kind: "movie", Path: "Movies", Title: "Alien"}, ""},
		{"flat folder unknown title", []string{"Movies/Heat (1995).mkv", "Movies/Heat (1995).nfo", "Movies/Alien (1979).mkv"},
			metadata.Query{Kind: "movie", Path: "Movies", Title: "Brazil"}, ""},
		{"flat folder wrong year", []string{"Movies/Dune (1984).mkv", "Movies/Dune (1984).nfo", "Movies/Dune (2021).mkv", "Movies/Dune (2021).nfo"},
			metadata.Query{Kind: "movie", Path: "Movies", Title: "Dune", Year: 2021}, "Movies/Dune (2021).nfo"},
		{"movie path is the file", []string{"Movies/Heat (1995).mkv", "Movies/Heat (1995).nfo", "Movies/Alien (1979).mkv"},
			metadata.Query{Kind: "movie", Path: "Movies/Heat (1995).mkv"}, "Movies/Heat (1995).nfo"},
		{"extras do not make a folder flat", []string{"Heat/Heat (1995).mkv", "Heat/Heat (1995)-trailer.mkv", "Heat/movie.nfo"},
			metadata.Query{Kind: "movie", Path: "Heat", Title: "Heat"}, "Heat/movie.nfo"},
		{"DVD folder", []string{"Heat/VIDEO_TS/VIDEO_TS.IFO", "Heat/VIDEO_TS/VIDEO_TS.nfo"},
			metadata.Query{Kind: "movie", Path: "Heat", Title: "Heat"}, "Heat/VIDEO_TS/VIDEO_TS.nfo"},
		{"tvshow.nfo", []string{"Lost/tvshow.nfo"}, metadata.Query{Kind: "series", Path: "Lost"}, "Lost/tvshow.nfo"},
		{"season.nfo", []string{"Lost/Season 1/Season.NFO"}, metadata.Query{Kind: "season", Path: "Lost/Season 1"}, "Lost/Season 1/Season.NFO"},
		{"episode nfo", []string{"Lost/S1/Lost.S01E01.mkv", "Lost/S1/Lost.S01E01.nfo"},
			metadata.Query{Kind: "episode", Path: "Lost/S1/Lost.S01E01.mkv"}, "Lost/S1/Lost.S01E01.nfo"},
		{"album.nfo", []string{"Radiohead/OK Computer/album.nfo"}, metadata.Query{Kind: "album", Path: "Radiohead/OK Computer"}, "Radiohead/OK Computer/album.nfo"},
		{"artist.nfo", []string{"Radiohead/artist.nfo"}, metadata.Query{Kind: "artist", Path: "Radiohead"}, "Radiohead/artist.nfo"},
		{"missing", []string{"Radiohead/x.mp3"}, metadata.Query{Kind: "artist", Path: "Radiohead"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]string{}
			for _, f := range tt.files {
				m[f] = ""
			}
			root := mkTree(t, m)
			q := tt.q
			q.Path = filepath.Join(root, filepath.FromSlash(q.Path))
			if got := rel(root, FindNFO(q)); got != tt.want {
				t.Errorf("FindNFO = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindImages(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		q     metadata.Query
		want  []string
	}{
		{"dedicated movie folder", []string{
			"Heat (1995)/Heat (1995).mkv", "Heat (1995)/Folder.JPG", "Heat (1995)/poster.png", "Heat (1995)/Heat (1995)-poster.jpg",
			"Heat (1995)/fanart.jpg", "Heat (1995)/backdrop2.jpg", "Heat (1995)/backdrop1.jpg", "Heat (1995)/extrafanart/b.jpg",
			"Heat (1995)/extrafanart/a.jpg", "Heat (1995)/extrafanart/notes.txt", "Heat (1995)/clearlogo.png", "Heat (1995)/logo.png",
			"Heat (1995)/banner.jpg", "Heat (1995)/landscape.jpg", "Heat (1995)/thumb.jpg", "Heat (1995)/random.jpg",
		}, metadata.Query{Kind: "movie", Path: "Heat (1995)", Title: "Heat"}, []string{
			"poster:Heat (1995)/poster.png", "poster:Heat (1995)/Folder.JPG", "poster:Heat (1995)/Heat (1995)-poster.jpg",
			"backdrop:Heat (1995)/fanart.jpg", "backdrop:Heat (1995)/backdrop1.jpg", "backdrop:Heat (1995)/backdrop2.jpg",
			"backdrop:Heat (1995)/extrafanart/a.jpg", "backdrop:Heat (1995)/extrafanart/b.jpg",
			"logo:Heat (1995)/logo.png", "logo:Heat (1995)/clearlogo.png",
			"banner:Heat (1995)/banner.jpg",
			"thumb:Heat (1995)/landscape.jpg", "thumb:Heat (1995)/thumb.jpg",
		}},
		{"flat movie folder", []string{
			"Movies/Heat (1995).mkv", "Movies/Alien (1979).mkv", "Movies/poster.jpg", "Movies/fanart.jpg",
			"Movies/Alien (1979)-poster.jpg", "Movies/alien (1979)-FANART.jpg", "Movies/Heat (1995)-poster.jpg", "Movies/Alien (1979).png",
		}, metadata.Query{Kind: "movie", Path: "Movies", Title: "Alien", Year: 1979}, []string{
			"poster:Movies/Alien (1979)-poster.jpg", "poster:Movies/Alien (1979).png", "backdrop:Movies/alien (1979)-FANART.jpg",
		}},
		{"flat folder with stacks is still dedicated", []string{
			"Heat/Heat (1995) cd1.avi", "Heat/Heat (1995) cd2.avi", "Heat/poster.jpg", "Heat/Heat (1995)-fanart.jpg",
		}, metadata.Query{Kind: "movie", Path: "Heat", Title: "Heat"}, []string{
			"poster:Heat/poster.jpg", "backdrop:Heat/Heat (1995)-fanart.jpg",
		}},
		{"series folder", []string{
			"Lost/poster.jpg", "Lost/fanart.jpg", "Lost/logo.png", "Lost/banner.jpg", "Lost/landscape.jpg", "Lost/season01-poster.jpg",
		}, metadata.Query{Kind: "series", Path: "Lost"}, []string{
			"poster:Lost/poster.jpg", "backdrop:Lost/fanart.jpg", "logo:Lost/logo.png", "banner:Lost/banner.jpg", "thumb:Lost/landscape.jpg",
		}},
		{"season posters", []string{
			"Lost/Season 1/folder.jpg", "Lost/Season01-Poster.JPG", "Lost/season1.jpg", "Lost/season01-fanart.jpg", "Lost/season01-banner.jpg",
			"Lost/season02-poster.jpg", "Lost/season-all-poster.jpg", "Lost/season-specials-poster.jpg",
		}, metadata.Query{Kind: "season", Path: "Lost/Season 1", IndexNumber: 1}, []string{
			"poster:Lost/Season 1/folder.jpg", "poster:Lost/Season01-Poster.JPG", "poster:Lost/season1.jpg", "poster:Lost/season-all-poster.jpg",
			"backdrop:Lost/season01-fanart.jpg", "banner:Lost/season01-banner.jpg",
		}},
		{"specials from folder name", []string{
			"Lost/Specials/x.mkv", "Lost/season-specials-poster.jpg", "Lost/season01-poster.jpg",
		}, metadata.Query{Kind: "season", Path: "Lost/Specials"}, []string{
			"poster:Lost/season-specials-poster.jpg",
		}},
		{"season number from folder", []string{"Lost/Season 3/x.mkv", "Lost/season03-poster.jpg"},
			metadata.Query{Kind: "season", Path: "Lost/Season 3"}, []string{"poster:Lost/season03-poster.jpg"}},
		{"episode", []string{
			"Lost/S1/Lost.S01E01.mkv", "Lost/S1/Lost.S01E01-thumb.jpg", "Lost/S1/lost.s01e01.PNG", "Lost/S1/Lost.S01E02-thumb.jpg",
			"Lost/S1/metadata/Lost.S01E01.jpg",
		}, metadata.Query{Kind: "episode", Path: "Lost/S1/Lost.S01E01.mkv"}, []string{
			"thumb:Lost/S1/Lost.S01E01-thumb.jpg", "thumb:Lost/S1/lost.s01e01.PNG", "thumb:Lost/S1/metadata/Lost.S01E01.jpg",
		}},
		{"album", []string{
			"OK/01.flac", "OK/folder.jpg", "OK/cover.png", "OK/AlbumArtSmall.jpg", "OK/AlbumArt_{ABC}_Large.jpg", "OK/fanart.jpg",
		}, metadata.Query{Kind: "album", Path: "OK"}, []string{
			"poster:OK/cover.png", "poster:OK/folder.jpg", "poster:OK/AlbumArt_{ABC}_Large.jpg", "poster:OK/AlbumArtSmall.jpg", "backdrop:OK/fanart.jpg",
		}},
		{"artist", []string{"DP/artist.jpg", "DP/folder.jpg", "DP/fanart.jpg", "DP/logo.png"},
			metadata.Query{Kind: "artist", Path: "DP"}, []string{
				"poster:DP/artist.jpg", "poster:DP/folder.jpg", "backdrop:DP/fanart.jpg", "logo:DP/logo.png",
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]string{}
			for _, f := range tt.files {
				m[f] = ""
			}
			root := mkTree(t, m)
			q := tt.q
			q.Path = filepath.Join(root, filepath.FromSlash(q.Path))
			got := FindImages(q)
			for _, im := range got {
				if !im.Local || !filepath.IsAbs(im.URL) {
					t.Errorf("image %+v: want Local and absolute URL", im)
				}
			}
			eq(t, "images", imgs(root, got), tt.want)
		})
	}
}

func TestImageSize(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 20, 30))); err != nil {
		t.Fatal(err)
	}
	root := mkTree(t, map[string]string{"Heat/Heat.mkv": "", "Heat/poster.png": buf.String(), "Heat/fanart.webp": ""})
	got := FindImages(metadata.Query{Kind: "movie", Path: filepath.Join(root, "Heat"), Title: "Heat"})
	if len(got) != 2 {
		t.Fatalf("images = %+v", got)
	}
	eq(t, "poster size", [2]int{got[0].Width, got[0].Height}, [2]int{20, 30})
	eq(t, "webp size", [2]int{got[1].Width, got[1].Height}, [2]int{0, 0})
}

func TestLocal(t *testing.T) {
	nfo, err := os.ReadFile(filepath.Join("testdata", "movie_kodi.nfo"))
	if err != nil {
		t.Fatal(err)
	}
	root := mkTree(t, map[string]string{
		"Amelie (2001)/Amelie (2001).mkv": "", "Amelie (2001)/movie.nfo": string(nfo), "Amelie (2001)/poster.jpg": "",
		"OnlyArt/OnlyArt.mkv": "", "OnlyArt/poster.jpg": "",
		"Nothing/Nothing.mkv": "",
		"BadNFO/BadNFO.mkv":   "", "BadNFO/movie.nfo": "garbage",
		"Lost/S1/Lost.S01E01-E02.mkv": "", "Lost/S1/Lost.S01E01-E02.nfo": mustRead(t, "episode_multi.nfo"),
	})
	p := New()
	ctx := context.Background()

	r, err := p.Local(ctx, metadata.Query{Kind: "movie", Path: filepath.Join(root, "Amelie (2001)"), Title: "Amelie"})
	if err != nil || r == nil {
		t.Fatalf("Local = %v, %v", r, err)
	}
	eq(t, "Title", r.Title, "Le Fabuleux Destin d'Amélie Poulain")
	if len(r.Images) != 4 || !r.Images[0].Local || r.Images[0].URL != filepath.Join(root, "Amelie (2001)", "poster.jpg") || r.Images[1].Local {
		t.Errorf("Images = %+v", r.Images)
	}

	r, err = p.Local(ctx, metadata.Query{Kind: "movie", Path: filepath.Join(root, "OnlyArt"), Title: "OnlyArt"})
	if err != nil || r == nil || len(r.Images) != 1 || r.Title != "" {
		t.Errorf("images only: %+v, %v", r, err)
	}
	r, err = p.Local(ctx, metadata.Query{Kind: "movie", Path: filepath.Join(root, "BadNFO"), Title: "BadNFO"})
	if err != nil || r != nil {
		t.Errorf("bad nfo: %+v, %v", r, err)
	}
	r, err = p.Local(ctx, metadata.Query{Kind: "movie", Path: filepath.Join(root, "Nothing"), Title: "Nothing"})
	if err != nil || r != nil {
		t.Errorf("nothing: %+v, %v", r, err)
	}
	r, err = p.Local(ctx, metadata.Query{Kind: "episode", Path: filepath.Join(root, "Lost/S1/Lost.S01E01-E02.mkv")})
	if err != nil || r == nil || r.IndexNumber != 1 || r.Children[2] == nil {
		t.Errorf("multi-episode: %+v, %v", r, err)
	}
	for _, q := range []metadata.Query{
		{Kind: "season", Path: ""},
		{Kind: "artist", Path: ""},
		{Kind: "track", Path: filepath.Join(root, "Nothing")},
		{Kind: "movie", Path: filepath.Join(root, "does-not-exist")},
	} {
		if r, err := p.Local(ctx, q); r != nil || err != nil {
			t.Errorf("Local(%+v) = %+v, %v; want nil, nil", q, r, err)
		}
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Local(cctx, metadata.Query{Kind: "movie", Path: root}); err == nil {
		t.Error("canceled context: want error")
	}
}

func mustRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestProvider(t *testing.T) {
	p := New()
	eq(t, "Name", p.Name(), "nfo")
	for _, k := range []string{"movie", "series", "season", "episode", "artist", "album"} {
		if !p.Supports(k) {
			t.Errorf("Supports(%q) = false", k)
		}
	}
	if p.Supports("track") || p.Supports("") {
		t.Error("Supports(track) = true")
	}
}
