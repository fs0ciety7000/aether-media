package naming

import (
	"regexp"
	"strconv"
	"strings"
)

// Track is the information parsed from a music file path. It is a fallback
// for files without usable tags.
type Track struct {
	Artist, Album, Title string
	Disc, Number, Year   int
}

var (
	discFolderRe = regexp.MustCompile(`(?i)^(?:cd|disc|disk|disque|disco|vol|volume|medium|vinyl|side)[ ._-]*(\d{1,3})(?:[^\d]|$)`)
	// "1-03 Title", "2-11. Title"
	discTrackRe = regexp.MustCompile(`^(\d{1,2})-(\d{1,3})(?:[ ._]*[-.][ ._]*|[ ._]+)(.+)$`)
	// "01. Title", "03 - Title", "01 Title", "(03) Title", "01_Title"
	trackNumRe = regexp.MustCompile(`^[(\[]?(\d{1,3})[)\]]?(?:[ _]*[-.)][ _]*|[ _]+)(.+)$`)
	// "03.flac"
	trackOnlyRe = regexp.MustCompile(`^(\d{1,3})$`)
	// "Artist - Album - 03 - Title"
	artistAlbumNumRe = regexp.MustCompile(`^(.+?) - (.+?) - (\d{1,3}) - (.+)$`)
	// "Artist - 03 - Title"
	artistNumRe = regexp.MustCompile(`^(.+?) - (\d{1,3}) - (.+)$`)
	// "Artist - 1-03 - Title"
	artistDiscNumRe = regexp.MustCompile(`^(.+?) - (\d{1,2})-(\d{1,3}) - (.+)$`)

	albumYearPrefixRe = regexp.MustCompile(`^[\[(]?((?:19|20)\d{2})[\])]?\s*(?:-|–|\.)?\s*(.+)$`)
	albumYearSuffixRe = regexp.MustCompile(`^(.+?)\s*[\[(]((?:19|20)\d{2})[\])]$`)
	audioJunkGroupRe  = regexp.MustCompile(`(?i)\s*[\[(](?:[^\])]*\b(?:flac|mp3|aac|alac|ogg|opus|320|256|v0|v2|kbps|\d+\s*bit|\d+(?:\.\d+)?\s*khz|web|cd|vinyl|lossless|hi-?res|24-\d+|16-\d+)\b[^\])]*)[\])]`)
)

var genericMusicFolders = set("music", "musique", "musik", "audio", "albums", "album", "mp3", "flac", "library",
	"media", "medias", "downloads", "lossless", "artists", "home", "mnt", "srv", "data", "volume1", "share")

// ParseTrack parses a music file path such as
// "Artist/Album (Year)/CD2/03 - Title.flac", "01. Title.mp3",
// "1-03 Title.m4a" (disc-track) or "Artist - Album - 03 - Title.mp3".
// Tags read from the file should be preferred over this result.
func ParseTrack(path string) Track {
	var t Track
	comps := splitPath(path)
	if len(comps) == 0 {
		return t
	}
	name := strings.TrimSpace(strings.ReplaceAll(stripKnownExt(comps[len(comps)-1]), "_", " "))
	parents := comps[:len(comps)-1]

	i := len(parents) - 1
	if i >= 0 {
		if m := discFolderRe.FindStringSubmatch(strings.TrimSpace(parents[i])); m != nil {
			t.Disc, _ = strconv.Atoi(m[1])
			i--
		}
	}
	var folderArtist string
	if i >= 0 {
		var artist string
		artist, t.Album, t.Year = parseAlbumFolder(parents[i])
		if i-1 >= 0 && isArtistFolder(parents[i-1]) {
			folderArtist = strings.TrimSpace(parents[i-1])
		}
		if artist != "" {
			// "Artist - Album (Year)" wins over the grand-parent folder.
			folderArtist = artist
		}
	}
	t.Artist = folderArtist

	switch {
	case trackOnlyRe.MatchString(name):
		t.Number, _ = strconv.Atoi(name)
	case artistDiscNumRe.MatchString(name):
		m := artistDiscNumRe.FindStringSubmatch(name)
		t.Artist = m[1]
		t.Disc, _ = strconv.Atoi(m[2])
		t.Number, _ = strconv.Atoi(m[3])
		t.Title = m[4]
	case artistAlbumNumRe.MatchString(name):
		m := artistAlbumNumRe.FindStringSubmatch(name)
		t.Artist, t.Number = m[1], atoi(m[3])
		if t.Album == "" {
			t.Album = m[2]
		}
		t.Title = m[4]
	case artistNumRe.MatchString(name) && !digitsRe.MatchString(strings.TrimSpace(strings.SplitN(name, " - ", 2)[0])):
		m := artistNumRe.FindStringSubmatch(name)
		t.Artist, t.Number, t.Title = m[1], atoi(m[2]), m[3]
	case discTrackRe.MatchString(name):
		m := discTrackRe.FindStringSubmatch(name)
		t.Disc, t.Number = atoi(m[1]), atoi(m[2])
		t.Title = trackTitle(m[3], &t)
	case trackNumRe.MatchString(name):
		m := trackNumRe.FindStringSubmatch(name)
		t.Number = atoi(m[1])
		t.Title = trackTitle(m[2], &t)
	default:
		if a, b, ok := strings.Cut(name, " - "); ok && (t.Artist == "" || strings.EqualFold(a, t.Artist)) {
			t.Artist, t.Title = strings.TrimSpace(a), strings.TrimSpace(b)
		} else {
			t.Title = name
		}
	}
	t.Title = strings.TrimSpace(t.Title)
	t.Artist = strings.TrimSpace(t.Artist)
	return t
}

// trackTitle strips an "Artist - " prefix from a numbered track title when
// it matches the known artist (or there is no folder context at all).
func trackTitle(rest string, t *Track) string {
	rest = strings.TrimSpace(rest)
	if a, b, ok := strings.Cut(rest, " - "); ok {
		if t.Artist != "" && strings.EqualFold(strings.TrimSpace(a), t.Artist) {
			return strings.TrimSpace(b)
		}
		if t.Artist == "" && t.Album == "" {
			t.Artist = strings.TrimSpace(a)
			return strings.TrimSpace(b)
		}
	}
	return rest
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func isArtistFolder(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n != "" && !genericMusicFolders[n] && !strings.HasSuffix(n, ":")
}

// parseAlbumFolder parses "Album (Year)", "Year - Album", "[Year] Album",
// "Artist - Album (Year)" and "Artist - Year - Album" folder names.
func parseAlbumFolder(name string) (artist, album string, year int) {
	s := strings.TrimSpace(strings.ReplaceAll(name, "_", " "))
	for {
		r := strings.TrimSpace(audioJunkGroupRe.ReplaceAllString(s, ""))
		if r == s || r == "" {
			break
		}
		s = r
	}
	if m := albumYearSuffixRe.FindStringSubmatch(s); m != nil {
		year, s = atoi(m[2]), strings.TrimSpace(m[1])
	} else if m := albumYearPrefixRe.FindStringSubmatch(s); m != nil && strings.TrimSpace(m[2]) != "" {
		year, s = atoi(m[1]), strings.TrimSpace(m[2])
	}
	parts := strings.Split(s, " - ")
	switch {
	case len(parts) >= 3 && yearTokenRe.MatchString(strings.TrimSpace(parts[1])):
		artist = strings.TrimSpace(parts[0])
		if year == 0 {
			year = atoi(strings.TrimSpace(parts[1]))
		}
		album = strings.TrimSpace(strings.Join(parts[2:], " - "))
	case len(parts) >= 2 && yearTokenRe.MatchString(strings.TrimSpace(parts[0])):
		if year == 0 {
			year = atoi(strings.TrimSpace(parts[0]))
		}
		album = strings.TrimSpace(strings.Join(parts[1:], " - "))
	case len(parts) >= 2:
		artist = strings.TrimSpace(parts[0])
		album = strings.TrimSpace(strings.Join(parts[1:], " - "))
	default:
		album = s
	}
	return artist, album, year
}
