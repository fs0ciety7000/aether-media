package naming

import (
	"regexp"
	"strings"
)

var (
	// "Movie-trailer", "Movie.sample", "Movie - Trailer 2", "Movie_trailer"
	extraSepSuffixRe = regexp.MustCompile(`(?i)(?:\s*-\s*|[._])(trailer|sample|teaser)\d*$`)
	// Plex/Jellyfin "-featurette", "-behindthescenes", "-deleted"...
	extraDashSuffixRe = regexp.MustCompile(`(?i)-(scene|clip|behindthescenes|deleted|deletedscene|featurette|short|interview|other|extra)\d*$`)
	extraExactRe      = regexp.MustCompile(`(?i)^(trailer|sample|teaser)\d*$`)
	samplePrefixRe    = regexp.MustCompile(`(?i)^sample[\s._-]`)
	extraPunctRe      = regexp.MustCompile(`[\s._-]+`)
)

var extraSuffixTypes = map[string]ExtraType{
	"trailer": ExtraTrailer, "teaser": ExtraTrailer, "sample": ExtraSample, "scene": ExtraScene, "clip": ExtraClip,
	"behindthescenes": ExtraBehindTheScenes, "deleted": ExtraDeleted, "deletedscene": ExtraDeleted,
	"featurette": ExtraFeaturette, "short": ExtraShort, "interview": ExtraInterview, "other": ExtraOther,
	"extra": ExtraOther,
}

// extraFolders maps normalized folder names (lower case, no separators) to
// their extra type.
var extraFolders = map[string]ExtraType{
	"trailers": ExtraTrailer, "trailer": ExtraTrailer, "bandesannonces": ExtraTrailer,
	"featurettes": ExtraFeaturette, "featurette": ExtraFeaturette,
	"behindthescenes": ExtraBehindTheScenes, "makingof": ExtraBehindTheScenes,
	"deletedscenes": ExtraDeleted, "deleted": ExtraDeleted, "scenescoupees": ExtraDeleted,
	"interviews": ExtraInterview, "interview": ExtraInterview,
	"scenes": ExtraScene, "shorts": ExtraShort, "clips": ExtraClip,
	"extras": ExtraOther, "extra": ExtraOther, "bonus": ExtraOther, "bonuses": ExtraOther, "other": ExtraOther,
	"others": ExtraOther, "featurettesandextras": ExtraOther,
	"sample": ExtraSample, "samples": ExtraSample,
}

func normalizeFolderKey(name string) string {
	return foldDiacritics(strings.ToLower(extraPunctRe.ReplaceAllString(strings.TrimSpace(name), "")))
}

// ClassifyExtra reports whether path is an extra, following the Jellyfin,
// Plex and Kodi conventions: a "-trailer" / ".sample" style suffix,
// "trailer.mkv" / "sample.mkv" files, "sample-..." scene samples, Plex
// suffixes ("-featurette", "-behindthescenes", "-deleted", "-interview",
// "-scene", "-short", "-clip", "-other"), or an immediate parent folder named
// Trailers, Featurettes, Behind The Scenes, Deleted Scenes, Interviews,
// Scenes, Shorts, Clips, Extras, Other or Sample(s).
func ClassifyExtra(path string) ExtraType {
	comps := splitPath(path)
	if len(comps) == 0 {
		return ExtraNone
	}
	if len(comps) >= 2 {
		if t := extraFolders[normalizeFolderKey(comps[len(comps)-2])]; t != ExtraNone {
			return t
		}
	}
	name := strings.TrimSpace(stripKnownExt(comps[len(comps)-1]))
	if m := extraExactRe.FindStringSubmatch(name); m != nil {
		return extraSuffixTypes[strings.ToLower(m[1])]
	}
	if m := extraSepSuffixRe.FindStringSubmatch(name); m != nil {
		return extraSuffixTypes[strings.ToLower(m[1])]
	}
	if m := extraDashSuffixRe.FindStringSubmatch(name); m != nil {
		return extraSuffixTypes[strings.ToLower(m[1])]
	}
	if samplePrefixRe.MatchString(name) {
		return ExtraSample
	}
	return ExtraNone
}

// stripExtraSuffix removes an extra suffix ("-trailer", ".sample") or a
// "sample-" prefix from a name without extension.
func stripExtraSuffix(name string) string {
	if extraExactRe.MatchString(strings.TrimSpace(name)) {
		return ""
	}
	if loc := extraSepSuffixRe.FindStringIndex(name); loc != nil {
		return name[:loc[0]]
	}
	if loc := extraDashSuffixRe.FindStringIndex(name); loc != nil {
		return name[:loc[0]]
	}
	if loc := samplePrefixRe.FindStringIndex(name); loc != nil {
		return name[loc[1]:]
	}
	return name
}
