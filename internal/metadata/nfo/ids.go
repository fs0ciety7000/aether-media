package nfo

import (
	"regexp"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

var (
	imdbRe      = regexp.MustCompile(`\b(tt\d{6,10})\b`)
	imdbBareRe  = regexp.MustCompile(`^(?:tt)?(\d{6,10})$`)
	tmdbURLRe   = regexp.MustCompile(`(?i)themoviedb\.org/(?:\d+/)?(movie|tv|collection)/(\d+)`)
	tvdbIDRe    = regexp.MustCompile(`(?i)thetvdb\.com/\S*?[?&](?:series)?id=(\d+)`)
	tvdbPathRe  = regexp.MustCompile(`(?i)thetvdb\.com/(?:dereferrer/)?series/(\d+)\b`)
	mbURLRe     = regexp.MustCompile(`(?i)musicbrainz\.org/(artist|release-group|release|recording)/([0-9a-f-]{36})`)
	numericIDRe = regexp.MustCompile(`^\d+$`)
)

// normIMDb returns an IMDb id in its "tt0123456" form, "" if s is not one.
func normIMDb(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if m := imdbBareRe.FindStringSubmatch(s); m != nil {
		return "tt" + m[1]
	}
	if m := imdbRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// setID stores v under key unless a value is already known.
func setID(ids metadata.ProviderIDs, key, v string) {
	v = strings.TrimSpace(v)
	if v == "" || ids[key] != "" {
		return
	}
	switch key {
	case metadata.IDImdb:
		if v = normIMDb(v); v == "" {
			return
		}
	case metadata.IDTmdb, metadata.IDTvdb, metadata.IDTmdbCollection:
		if !numericIDRe.MatchString(v) || v == "0" {
			return
		}
	}
	ids[key] = v
}

// idsFromText extracts provider ids from free text: IMDb, TMDb, TheTVDB and
// MusicBrainz links as written in URL-only NFOs or after the XML.
func idsFromText(ids metadata.ProviderIDs, text string) {
	for _, m := range imdbRe.FindAllStringSubmatch(text, 1) {
		setID(ids, metadata.IDImdb, m[1])
	}
	for _, m := range tmdbURLRe.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(m[1], "collection") {
			setID(ids, metadata.IDTmdbCollection, m[2])
		} else {
			setID(ids, metadata.IDTmdb, m[2])
		}
	}
	for _, re := range []*regexp.Regexp{tvdbIDRe, tvdbPathRe} {
		if m := re.FindStringSubmatch(text); m != nil {
			setID(ids, metadata.IDTvdb, m[1])
		}
	}
	for _, m := range mbURLRe.FindAllStringSubmatch(text, -1) {
		key := map[string]string{
			"artist":        metadata.IDMusicBrainzArtist,
			"release-group": metadata.IDMusicBrainzRelGrp,
			"release":       metadata.IDMusicBrainzRelease,
			"recording":     metadata.IDMusicBrainzTrack,
		}[strings.ToLower(m[1])]
		setID(ids, key, strings.ToLower(m[2]))
	}
}
