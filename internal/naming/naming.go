// Package naming parses media file and folder names into structured
// information (titles, years, season/episode numbers, provider ids, extras,
// multi-part stacks, music track numbers...).
//
// The package is pure: it never touches the filesystem or a database. Every
// function works on path strings only, and both '/' and '\' are accepted as
// path separators so that Windows paths parse the same way as POSIX ones.
//
// It understands the naming schemes used by Radarr, Sonarr and Lidarr, scene
// release names, and the Kodi / Jellyfin / Plex library conventions.
package naming

import (
	"regexp"
	"strings"
)

// ProviderIDs holds metadata provider identifiers found in a file or folder
// name, such as "{tmdb-123}", "[tmdbid-123]", "{imdb-tt0113277}",
// "[imdbid-tt0113277]", "{tvdb-81189}" or "[tvdbid-81189]".
type ProviderIDs struct{ TMDB, IMDB, TVDB string }

// IsZero reports whether no identifier is set.
func (p ProviderIDs) IsZero() bool { return p.TMDB == "" && p.IMDB == "" && p.TVDB == "" }

// merge fills the empty fields of p with the values of o.
func (p *ProviderIDs) merge(o ProviderIDs) {
	if p.TMDB == "" {
		p.TMDB = o.TMDB
	}
	if p.IMDB == "" {
		p.IMDB = o.IMDB
	}
	if p.TVDB == "" {
		p.TVDB = o.TVDB
	}
}

// ExtraType classifies a video that is an extra (bonus material) of a movie
// or series. The empty value means the file is not an extra.
type ExtraType string

// Known extra types.
const (
	ExtraNone            ExtraType = ""
	ExtraTrailer         ExtraType = "trailer"
	ExtraSample          ExtraType = "sample"
	ExtraFeaturette      ExtraType = "featurette"
	ExtraBehindTheScenes ExtraType = "behindthescenes"
	ExtraDeleted         ExtraType = "deleted"
	ExtraInterview       ExtraType = "interview"
	ExtraScene           ExtraType = "scene"
	ExtraShort           ExtraType = "short"
	ExtraClip            ExtraType = "clip"
	ExtraOther           ExtraType = "other"
)

var (
	// idTagRe matches provider id tags: {tmdb-123}, [tmdbid-123], [imdbid=tt123], (tvdb 123)...
	idTagRe = regexp.MustCompile(`(?i)[\[{(]\s*(tmdb|tmdbid|imdb|imdbid|tvdb|tvdbid)\s*[-=: ]\s*([a-z0-9]+)\s*[\]})]`)
	// editionTagRe matches the Radarr / Plex edition tag: {edition-Director's Cut}.
	editionTagRe = regexp.MustCompile(`(?i)\{\s*edition-([^}]*)\}`)
	digitsRe     = regexp.MustCompile(`^\d+$`)
	imdbRe       = regexp.MustCompile(`^tt\d{5,}$`)
)

// ParseProviderIDs extracts the provider id tags present in name.
func ParseProviderIDs(name string) ProviderIDs {
	ids, _ := extractIDs(name)
	return ids
}

// extractIDs returns the provider ids found in s and s with the valid id
// tags replaced by a space.
func extractIDs(s string) (ProviderIDs, string) {
	var ids ProviderIDs
	if !strings.ContainsAny(s, "[{(") {
		return ids, s
	}
	out := idTagRe.ReplaceAllStringFunc(s, func(tag string) string {
		m := idTagRe.FindStringSubmatch(tag)
		kind, val := strings.ToLower(m[1]), strings.ToLower(m[2])
		switch kind {
		case "tmdb", "tmdbid":
			if !digitsRe.MatchString(val) {
				return tag
			}
			if ids.TMDB == "" {
				ids.TMDB = val
			}
		case "tvdb", "tvdbid":
			if !digitsRe.MatchString(val) {
				return tag
			}
			if ids.TVDB == "" {
				ids.TVDB = val
			}
		default:
			if !imdbRe.MatchString(val) {
				return tag
			}
			if ids.IMDB == "" {
				ids.IMDB = val
			}
		}
		return " "
	})
	return ids, out
}

// extractEditionTag returns the content of a "{edition-...}" tag and s
// without it.
func extractEditionTag(s string) (string, string) {
	m := editionTagRe.FindStringSubmatchIndex(s)
	if m == nil {
		return "", s
	}
	ed := strings.TrimSpace(s[m[2]:m[3]])
	return ed, s[:m[0]] + " " + s[m[1]:]
}

// splitPath splits a path on both '/' and '\' and drops empty components.
func splitPath(p string) []string {
	c := strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
	for i := range c {
		c[i] = capLen(c[i])
	}
	return c
}

// baseName returns the last path component (handles both separators).
func baseName(p string) string {
	c := splitPath(p)
	if len(c) == 0 {
		return ""
	}
	return c[len(c)-1]
}

// fileExt returns the lower-cased extension of the last path component,
// without the dot, or "" when there is none.
func fileExt(p string) string {
	name := baseName(p)
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}

// stripKnownExt removes the extension of name when it is a known media,
// subtitle, image or sidecar extension. Unknown "extensions" are kept so that
// names like "Last.Call.for.Nowhere.WEB-DL.1080p" are not truncated.
func stripKnownExt(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name
	}
	ext := strings.ToLower(name[i+1:])
	if videoExts[ext] || audioExts[ext] || subtitleExts[ext] || imageExts[ext] || sidecarExts[ext] {
		return name[:i]
	}
	return name
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

var (
	videoExts = set("mkv", "mp4", "m4v", "avi", "mov", "wmv", "ts", "m2ts", "mts", "webm", "mpg", "mpeg",
		"mpe", "m2v", "vob", "iso", "ogv", "ogm", "3gp", "3g2", "flv", "f4v", "divx", "xvid", "rmvb", "rm",
		"asf", "wtv", "dvr-ms", "mk3d", "y4m")
	audioExts = set("flac", "mp3", "m4a", "m4b", "aac", "ogg", "oga", "opus", "wav", "wma", "alac", "aiff",
		"aif", "ape", "wv", "dsf", "dff", "mka", "mpc", "tta", "mp2", "ac3", "dts", "weba")
	subtitleExts = set("srt", "ass", "ssa", "vtt", "sub", "idx", "sup", "smi", "sami", "ttml", "dfxp", "pgs")
	imageExts    = set("jpg", "jpeg", "png", "webp", "gif", "tbn", "bmp", "avif", "heic", "tif", "tiff")
	sidecarExts  = set("nfo", "strm", "txt", "nzb", "ifo", "bup", "torrent", "part", "sfv", "md5", "cue", "log", "m3u", "m3u8")
)

// IsVideo reports whether path has a video file extension (case-insensitive).
func IsVideo(path string) bool { return videoExts[fileExt(path)] }

// IsAudio reports whether path has an audio file extension (case-insensitive).
func IsAudio(path string) bool { return audioExts[fileExt(path)] }

// IsSubtitle reports whether path has a subtitle file extension (case-insensitive).
func IsSubtitle(path string) bool { return subtitleExts[fileExt(path)] }

// IsImage reports whether path has an image file extension (case-insensitive).
func IsImage(path string) bool { return imageExts[fileExt(path)] }
