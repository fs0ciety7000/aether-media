package library

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/naming"
)

// Library kinds.
const (
	KindMovies = "movies"
	KindShows  = "shows"
	KindMusic  = "music"
	KindPhotos = "photos"
	KindMixed  = "mixed"
)

// node is one level of the item tree derived from a file path. Pointers are
// nil when the value is unknown.
type node struct {
	key         string
	kind        string
	title       string
	year        int
	path        string
	index       *int
	indexEnd    *int
	parentIndex *int
	edition     string
	airDate     string
	ids         naming.ProviderIDs
}

// resolved is the item chain (root first, leaf last) for one media file.
type resolved struct {
	chain []node
	part  int
}

func intp(v int) *int { return &v }

// accepts reports whether a file belongs in a library of the given kind.
func accepts(kind, p string) bool {
	switch kind {
	case KindMovies, KindShows:
		return naming.IsVideo(p)
	case KindMusic:
		return naming.IsAudio(p)
	case KindMixed:
		return naming.IsVideo(p) || naming.IsAudio(p)
	}
	return false
}

// resolve maps a file to its item chain. ok is false for files that are
// recognised but not indexed (extras, unparseable episodes).
func resolve(libID int64, kind string, f file) (resolved, bool) {
	switch kind {
	case KindMovies:
		return resolveMovie(f)
	case KindShows:
		return resolveEpisode(f)
	case KindMusic:
		return resolveTrack(libID, f)
	case KindMixed:
		if naming.IsAudio(f.path) {
			return resolveTrack(libID, f)
		}
		if ep, ok := naming.ParseEpisode(f.path); ok && (len(ep.Episodes) > 0 || ep.AirDate != "") {
			return resolveEpisode(f)
		}
		return resolveMovie(f)
	}
	return resolved{}, false
}

func resolveMovie(f file) (resolved, bool) {
	if naming.ClassifyExtra(f.path) != "" {
		return resolved{}, false // extras are indexed with their movie in a later milestone
	}
	m := naming.ParseMovie(f.path)
	title := m.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))
	}
	dir := filepath.Dir(f.path)
	// In a dedicated "Title (Year)" folder (*arr layout) the folder name is
	// canonical; file names are often abbreviated release names.
	if dir != f.root {
		if fm := naming.ParseSeriesFolder(filepath.Base(dir)); fm.Title != "" && fm.Year > 0 && (m.Year == 0 || m.Year == fm.Year) {
			title, m.Year = fm.Title, fm.Year
			if m.IDs.IsZero() {
				m.IDs = fm.IDs
			}
		}
	}
	// Parts and versions of the same movie in one folder share an item.
	key := fmt.Sprintf("movie:%s|%s|%d|%s", dir, strings.ToLower(title), m.Year, strings.ToLower(m.Edition))
	return resolved{
		chain: []node{{
			key: key, kind: "movie", title: title, year: m.Year, path: dir,
			edition: m.Edition, ids: m.IDs,
		}},
		part: m.Part,
	}, true
}

func resolveEpisode(f file) (resolved, bool) {
	if naming.ClassifyExtra(f.path) != "" {
		return resolved{}, false
	}
	ep, ok := naming.ParseEpisode(f.path)
	if !ok {
		return resolved{}, false
	}
	segs := strings.Split(f.rel, "/")

	// Series: the first folder under the library root, else the parsed title.
	var series node
	if len(segs) >= 2 {
		dir := filepath.Join(f.root, filepath.FromSlash(segs[0]))
		s := naming.ParseSeriesFolder(segs[0])
		title := s.Title
		if title == "" {
			title = segs[0]
		}
		series = node{key: "series:" + dir, kind: "series", title: title, year: s.Year, path: dir, ids: s.IDs}
	} else {
		if ep.SeriesTitle == "" {
			return resolved{}, false
		}
		series = node{
			key:  "series:" + f.root + "#" + strings.ToLower(ep.SeriesTitle),
			kind: "series", title: ep.SeriesTitle, year: ep.Year, ids: ep.IDs,
		}
	}

	// Season: from the file name, else the folder, else 1 (absolute numbering).
	season := ep.Season
	seasonDir := ""
	if len(segs) >= 3 {
		if n, ok := naming.ParseSeasonFolder(segs[len(segs)-2]); ok {
			seasonDir = filepath.Dir(f.path)
			if season < 0 {
				season = n
			}
		}
	}
	if season < 0 && ep.AirDate != "" {
		season, _ = strconv.Atoi(ep.AirDate[:4])
	}
	if season < 0 {
		season = 1
	}
	seasonTitle := fmt.Sprintf("Saison %d", season)
	if season == 0 {
		seasonTitle = "Spéciaux"
	}
	seasonNode := node{
		key:  fmt.Sprintf("season:%s#%d", strings.TrimPrefix(series.key, "series:"), season),
		kind: "season", title: seasonTitle, path: seasonDir, index: intp(season),
	}

	e := node{key: "episode:" + f.path, kind: "episode", path: f.path, parentIndex: intp(season), airDate: ep.AirDate, ids: ep.IDs}
	switch {
	case len(ep.Episodes) > 0:
		e.index = intp(ep.Episodes[0])
		if n := len(ep.Episodes); n > 1 {
			e.indexEnd = intp(ep.Episodes[n-1])
		}
	case ep.Absolute > 0:
		e.index = intp(ep.Absolute)
	}
	e.title = ep.Title
	if e.title == "" {
		switch {
		case e.index != nil:
			e.title = fmt.Sprintf("Épisode %d", *e.index)
		case ep.AirDate != "":
			e.title = ep.AirDate
		default:
			e.title = strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))
		}
	}
	return resolved{chain: []node{series, seasonNode, e}}, true
}

var discDir = regexp.MustCompile(`(?i)^(cd|dis[ck]|disque)\s*[-_.]?\s*(\d{1,2})$`)

func resolveTrack(libID int64, f file) (resolved, bool) {
	t := naming.ParseTrack(f.path)
	albumDir := filepath.Dir(f.path)
	disc := t.Disc
	if m := discDir.FindStringSubmatch(filepath.Base(albumDir)); m != nil {
		if disc == 0 {
			disc, _ = strconv.Atoi(m[2])
		}
		albumDir = filepath.Dir(albumDir)
	}
	// Folder layout fallback: <root>/<Artist>/<Album>/[CDn/]track.
	artist := t.Artist
	if artist == "" && albumDir != f.root && filepath.Dir(albumDir) != f.root {
		artist = filepath.Base(filepath.Dir(albumDir))
	}
	if artist == "" {
		artist = "Artiste inconnu"
	}
	album := t.Album
	if album == "" {
		if albumDir == f.root {
			album = "Album inconnu"
		} else {
			album = filepath.Base(albumDir)
		}
	}
	title := t.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))
	}

	artistNode := node{key: fmt.Sprintf("artist:%d:%s", libID, strings.ToLower(artist)), kind: "artist", title: artist}
	albumNode := node{key: "album:" + albumDir + "|" + strings.ToLower(album), kind: "album", title: album, year: t.Year, path: albumDir}
	track := node{key: "track:" + f.path, kind: "track", title: title, path: f.path}
	if t.Number > 0 {
		track.index = intp(t.Number)
	}
	if disc > 0 {
		track.parentIndex = intp(disc)
	}
	return resolved{chain: []node{artistNode, albumNode, track}}, true
}
