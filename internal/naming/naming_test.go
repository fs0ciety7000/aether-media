package naming

import (
	"strings"
	"testing"
)

var validExtras = map[ExtraType]struct{}{
	ExtraNone: {}, ExtraTrailer: {}, ExtraSample: {}, ExtraFeaturette: {}, ExtraBehindTheScenes: {},
	ExtraDeleted: {}, ExtraInterview: {}, ExtraScene: {}, ExtraShort: {}, ExtraClip: {}, ExtraOther: {},
}

func trimSpace(s string) string { return strings.TrimSpace(s) }

func TestClassifyExtra(t *testing.T) {
	cases := []struct {
		path string
		want ExtraType
	}{
		{"Movies/Heat (1995)/Heat (1995).mkv", ExtraNone},
		{"Movies/Heat (1995)/Heat (1995)-trailer.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/Heat (1995).trailer.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/Heat (1995)_trailer.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/Heat (1995) - Trailer.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/Heat (1995)-trailer2.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/trailer.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/Trailer.MP4", ExtraTrailer},
		{"Movies/Heat (1995)/trailer1.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/Trailers/Official Trailer.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/trailers/teaser.mkv", ExtraTrailer},
		{"Movies/Heat (1995)/Featurettes/Making of.mkv", ExtraFeaturette},
		{"Movies/Heat (1995)/Heat-featurette.mkv", ExtraFeaturette},
		{"Movies/Heat (1995)/Behind The Scenes/Shooting.mkv", ExtraBehindTheScenes},
		{"Movies/Heat (1995)/Heat-behindthescenes.mkv", ExtraBehindTheScenes},
		{"Movies/Heat (1995)/Deleted Scenes/Bank.mkv", ExtraDeleted},
		{"Movies/Heat (1995)/Heat-deleted.mkv", ExtraDeleted},
		{"Movies/Heat (1995)/Heat-deletedscene.mkv", ExtraDeleted},
		{"Movies/Heat (1995)/Interviews/Al Pacino.mkv", ExtraInterview},
		{"Movies/Heat (1995)/Heat-interview.mkv", ExtraInterview},
		{"Movies/Heat (1995)/Scenes/Shootout.mkv", ExtraScene},
		{"Movies/Heat (1995)/Heat-scene.mkv", ExtraScene},
		{"Movies/Heat (1995)/Shorts/Short film.mkv", ExtraShort},
		{"Movies/Heat (1995)/Heat-short.mkv", ExtraShort},
		{"Movies/Heat (1995)/Clips/Clip 1.mkv", ExtraClip},
		{"Movies/Heat (1995)/Heat-clip.mkv", ExtraClip},
		{"Movies/Heat (1995)/Extras/Gag reel.mkv", ExtraOther},
		{"Movies/Heat (1995)/Other/Gag reel.mkv", ExtraOther},
		{"Movies/Heat (1995)/Heat-other.mkv", ExtraOther},
		{`D:\Movies\Heat (1995)\Extras\Gag reel.mkv`, ExtraOther},
		{"Movies/Heat (1995)/Sample/heat.mkv", ExtraSample},
		{"Movies/Heat (1995)/heat.1995.1080p-sample.mkv", ExtraSample},
		{"Movies/Heat (1995)/sample-heat.1995.1080p.mkv", ExtraSample},
		{"Movies/Heat (1995)/sample.mkv", ExtraSample},
		{"Movies/Heat (1995)/Heat.1995.sample.mkv", ExtraSample},
		{"TV/Show/Season 1/Show - S01E05 - The Trailer.mkv", ExtraNone},
		{"Movies/The Sample (2010)/The Sample (2010).mkv", ExtraNone},
		{"Movies/Trailer Park Boys (2006)/Trailer Park Boys (2006).mkv", ExtraNone},
		{"Movies/Shorts (2009)/Shorts (2009).mkv", ExtraNone},
		{"", ExtraNone},
	}
	for _, tc := range cases {
		if got := ClassifyExtra(tc.path); got != tc.want {
			t.Errorf("ClassifyExtra(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestSortTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The Matrix", "matrix"},
		{"the matrix", "matrix"},
		{"A Beautiful Mind", "beautiful mind"},
		{"An American Werewolf in London", "american werewolf in london"},
		{"Le Fabuleux Destin d'Amélie Poulain", "fabuleux destin d'amelie poulain"},
		{"La Haine", "haine"},
		{"Les Misérables", "miserables"},
		{"L'Odyssée de Pi", "odyssee de pi"},
		{"L’Auberge espagnole", "auberge espagnole"},
		{"Un Prophète", "prophete"},
		{"Une Femme est une femme", "femme est une femme"},
		{"Der Untergang", "untergang"},
		{"Das Boot", "boot"},
		{"Die Welle", "welle"},
		{"Die Hard", "die hard"},
		{"El Laberinto del Fauno", "laberinto del fauno"},
		{"Los Olvidados", "olvidados"},
		{"Las Horas", "horas"},
		{"Il Postino", "postino"},
		{"The", "the"},
		{"A", "a"},
		{"Amélie", "amelie"},
		{"Æon Flux", "aeon flux"},
		{"Œdipe Roi", "oedipe roi"},
		{"1917", "1917"},
		{"2001 A Space Odyssey", "2001 a space odyssey"},
		{"The 400 Blows", "400 blows"},
		{"'Salem's Lot", "salem's lot"},
		{"...And Justice for All", "and justice for all"},
		{"  The   Big   Lebowski ", "big lebowski"},
		{"Theodore Rex", "theodore rex"},
		{"Annie Hall", "annie hall"},
		{"A.I. Artificial Intelligence", "a.i. artificial intelligence"},
		{"Ça", "ca"},
		{"Ève", "eve"},
		{"Straße", "strasse"},
	}
	for _, tc := range cases {
		if got := SortTitle(tc.in); got != tc.want {
			t.Errorf("SortTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Super movie 480p.mp4", "Super movie"},
		{"Super movie 480p", "Super movie"},
		{"Super movie Multi", "Super movie"},
		{"Super movie [480p]", "Super movie"},
		{"480 Super movie [tmdbid=12345]", "480 Super movie"},
		{"Crouching.Tiger.Hidden.Dragon.4k", "Crouching Tiger Hidden Dragon"},
		{"Crouching.Tiger.Hidden.Dragon.UltraHD", "Crouching Tiger Hidden Dragon"},
		{"Crouching.Tiger.Hidden.Dragon.UHD", "Crouching Tiger Hidden Dragon"},
		{"Crouching.Tiger.Hidden.Dragon.HDR", "Crouching Tiger Hidden Dragon"},
		{"Crouching.Tiger.Hidden.Dragon-HDC", "Crouching Tiger Hidden Dragon"},
		{"WALL-E", "WALL-E"},
		{"Spider-Man", "Spider-Man"},
		{"Crouching.Tiger.Hidden.Dragon.BDrip-HDC", "Crouching Tiger Hidden Dragon"},
		{"Crouching.Tiger.Hidden.Dragon.4K.UltraHD.HDR.BDrip-HDC", "Crouching Tiger Hidden Dragon"},
		{"Last.Call.for.Nowhere.WEB-DL.1080p", "Last Call for Nowhere"},
		{"[HorribleSubs] Made in Abyss - 13 [720p]", "Made in Abyss - 13"},
		{"[Tsundere] Kore wa Zombie Desu ka of the Dead [BDRip h264 1920x1080 FLAC]", "Kore wa Zombie Desu ka of the Dead"},
		{"[Erai-raws] Jujutsu Kaisen [720p][Multiple Subtitle]", "Jujutsu Kaisen"},
		{"Marvels.Agents.of.S.H.I.E.L.D.S01E01.720p", "Marvels Agents of S.H.I.E.L.D. S01E01"},
		{"Marvel's Agents of S.H.I.E.L.D.", "Marvel's Agents of S.H.I.E.L.D."},
		{"Mr.Robot", "Mr. Robot"},
		{"Movie_Name_With_Underscores", "Movie Name With Underscores"},
		{"Movie.Title.x264-GROUP", "Movie Title"},
		{"Movie Title HDTV x264-GROUP", "Movie Title"},
		{"Movie Title WEB-DL DD5.1 H.264-GROUP", "Movie Title"},
		{"Movie.Title.DDP5.1.Atmos", "Movie Title"},
		{"Movie Title TrueHD 7.1 Atmos", "Movie Title"},
		{"Movie Title HEVC 10bit", "Movie Title"},
		{"Movie Title AV1 Opus", "Movie Title"},
		{"Movie Title XviD", "Movie Title"},
		{"Movie Title DTS-HD MA", "Movie Title"},
		{"Movie Title EAC3", "Movie Title"},
		{"Movie Title DoVi", "Movie Title"},
		{"Movie Title HDR10", "Movie Title"},
		{"Movie.Title.MULTi.VFF.1080p", "Movie Title"},
		{"Movie.Title.VFQ.720p", "Movie Title"},
		{"Movie.Title.TRUEFRENCH.DVDRip", "Movie Title"},
		{"Movie.Title.FRENCH.BDRip", "Movie Title"},
		{"Movie.Title.PROPER.REPACK", "Movie Title"},
		{"Movie Title REMUX", "Movie Title"},
		{"Movie Title [YTS.MX]", "Movie Title"},
		{"Movie Title (2010)", "Movie Title (2010)"},
		{"The French Dispatch", "The French Dispatch"},
		{"Web of Lies", "Web of Lies"},
		{"Uncut Gems", "Uncut Gems"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := CleanTitle(tc.in); got != tc.want {
			t.Errorf("CleanTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFileTypes(t *testing.T) {
	cases := []struct {
		path                       string
		video, audio, sub, picture bool
	}{
		{"a.mkv", true, false, false, false},
		{"a.MP4", true, false, false, false},
		{"a.m4v", true, false, false, false},
		{"a.avi", true, false, false, false},
		{"a.mov", true, false, false, false},
		{"a.wmv", true, false, false, false},
		{"a.ts", true, false, false, false},
		{"a.m2ts", true, false, false, false},
		{"a.webm", true, false, false, false},
		{"a.mpg", true, false, false, false},
		{"a.mpeg", true, false, false, false},
		{"a.VOB", true, false, false, false},
		{"a.iso", true, false, false, false},
		{"a.ogv", true, false, false, false},
		{"a.3gp", true, false, false, false},
		{"a.flv", true, false, false, false},
		{"a.flac", false, true, false, false},
		{"a.mp3", false, true, false, false},
		{"a.M4A", false, true, false, false},
		{"a.aac", false, true, false, false},
		{"a.ogg", false, true, false, false},
		{"a.opus", false, true, false, false},
		{"a.wav", false, true, false, false},
		{"a.wma", false, true, false, false},
		{"a.alac", false, true, false, false},
		{"a.aiff", false, true, false, false},
		{"a.ape", false, true, false, false},
		{"a.wv", false, true, false, false},
		{"a.dsf", false, true, false, false},
		{"a.srt", false, false, true, false},
		{"a.fr.forced.ass", false, false, true, false},
		{"a.ssa", false, false, true, false},
		{"a.vtt", false, false, true, false},
		{"a.sub", false, false, true, false},
		{"a.idx", false, false, true, false},
		{"a.sup", false, false, true, false},
		{"a.jpg", false, false, false, true},
		{"a.JPEG", false, false, false, true},
		{"a.png", false, false, false, true},
		{"a.webp", false, false, false, true},
		{"a.gif", false, false, false, true},
		{"poster.tbn", false, false, false, true},
		{`C:\Movies\Heat\folder.jpg`, false, false, false, true},
		{"Movies/Heat.1995/movie.nfo", false, false, false, false},
		{"Movies/mkv", false, false, false, false},
		{".mkv", false, false, false, false},
		{"noext", false, false, false, false},
		{"", false, false, false, false},
		{"dir.mkv/file", false, false, false, false},
	}
	for _, tc := range cases {
		if got := IsVideo(tc.path); got != tc.video {
			t.Errorf("IsVideo(%q) = %v", tc.path, got)
		}
		if got := IsAudio(tc.path); got != tc.audio {
			t.Errorf("IsAudio(%q) = %v", tc.path, got)
		}
		if got := IsSubtitle(tc.path); got != tc.sub {
			t.Errorf("IsSubtitle(%q) = %v", tc.path, got)
		}
		if got := IsImage(tc.path); got != tc.picture {
			t.Errorf("IsImage(%q) = %v", tc.path, got)
		}
	}
}

func TestParseProviderIDs(t *testing.T) {
	cases := []struct {
		in   string
		want ProviderIDs
	}{
		{"Heat (1995) {tmdb-949}", ProviderIDs{TMDB: "949"}},
		{"Heat (1995) [tmdbid-949]", ProviderIDs{TMDB: "949"}},
		{"Heat (1995) [tmdbid=949]", ProviderIDs{TMDB: "949"}},
		{"Heat (1995) {imdb-tt0113277}", ProviderIDs{IMDB: "tt0113277"}},
		{"Heat (1995) [imdbid-tt0113277]", ProviderIDs{IMDB: "tt0113277"}},
		{"Heat (1995) [IMDBID-TT0113277]", ProviderIDs{IMDB: "tt0113277"}},
		{"Breaking Bad (2008) {tvdb-81189}", ProviderIDs{TVDB: "81189"}},
		{"Breaking Bad (2008) [tvdbid-81189]", ProviderIDs{TVDB: "81189"}},
		{"X {tmdb-1} {imdb-tt1234567} {tvdb-2}", ProviderIDs{TMDB: "1", IMDB: "tt1234567", TVDB: "2"}},
		{"X {tmdb-abc} [imdbid-123]", ProviderIDs{}},
		{"Heat (1995)", ProviderIDs{}},
	}
	for _, tc := range cases {
		if got := ParseProviderIDs(tc.in); got != tc.want {
			t.Errorf("ParseProviderIDs(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}
