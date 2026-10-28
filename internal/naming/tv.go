package naming

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Series is the information parsed from a series folder name.
type Series struct {
	Title string
	Year  int
	IDs   ProviderIDs
}

// Episode is the information parsed from an episode file path.
type Episode struct {
	SeriesTitle string // best-effort from the file name, else from the series folder
	Season      int    // -1 if unknown (absolute-numbered anime, daily shows outside a season folder); 0 = specials
	Episodes    []int  // one or more episode numbers (ranges are expanded)
	AirDate     string // "2024-03-15" for daily shows, "" otherwise
	Title       string // episode title after the numbering (cleaned), "" otherwise
	Absolute    int    // absolute number for anime-style "[Group] Show - 123 [1080p].mkv", 0 otherwise
	Year        int    // series year (file name, else series folder), 0 if unknown
	IDs         ProviderIDs
}

const (
	maxEpisodes   = 64
	maxRangeWidth = 40
)

var (
	// S01E02, s01.e02, S01xE02, S01x02, S2009E23, S01 E02, S01EP02
	seRe = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(s)(\d{1,4})[ ._-]?(?:x?e|x|ep)[ ._-]?(\d{1,4})`)
	// "Season 3 Episode 9", "Saison 1 Épisode 2"
	wordySERe = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])((?:season|saison|staffel|temporada|stagione|series)[ ._-]*(\d{1,4})[ ._-]*(?:,[ ._-]*)?(?:episode|épisode|episodio|folge|ep)[ ._-]*(\d{1,4}))`)
	// 1x02, 02x03, 2009x02
	nxRe = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(\d{1,2}|(?:19|20)\d{2})x(\d{1,3})(?:[^\p{N}]|$)`)
	// 2024.03.15 (normalized to spaces), 2024-03-15, 2018_03_24
	ymdRe = regexp.MustCompile(`(?:^|[^\p{N}])((19|20)\d{2})[ ._-](\d{1,2})[ ._-](\d{1,2})(?:[^\p{N}]|$)`)
	// 15.03.2024, 03-15-2024
	dmyRe = regexp.MustCompile(`(?:^|[^\p{N}])((\d{1,2})[ ._-](\d{1,2})[ ._-]((?:19|20)\d{2}))(?:[^\p{N}]|$)`)
	// Episode 16, Ep 02, E07, Épisode 3
	epOnlyRe = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])((?:episode|épisode|episodio|folge|ep|e)[ ._]*-?[ ._]*(\d{1,4}))(?:[^\p{L}\p{N}]|$)`)
	// "Show - 123", "Show - 03v2"
	dashNumRe = regexp.MustCompile(`(?i)\s-\s*((\d{1,4})(?:v\d{1,2})?)(?:[\s\[\(]|$)`)
	// "[21]", "[04v2]"
	bracketNumRe = regexp.MustCompile(`(?i)\[((\d{1,4})(?:v\d{1,2})?)\]`)
	// "One Piece 1001", "Show 05 [720p]"
	trailingNumRe = regexp.MustCompile(`(?i)^(.*?\S)[\s-]+((\d{1,4})(?:v\d{1,2})?)\s*(?:(?:\[[^\]]*\]|\([^)]*\))\s*)*$`)
	// "16 Some Title", "02 - blah", "02-04 - blah", "2. Infestation" (normalized), "02"
	leadingNumRe = regexp.MustCompile(`^((\d{1,3})(?:-(\d{1,3}))?)(?:$|\s*-\s*|\s+)`)
	// a time of day after a date: "19 00 00"
	timeRe = regexp.MustCompile(`^\s*\d{2}[ :._-]\d{2}(?:[ :._-]\d{2})?(?:\s|$)`)
	// continuation of a multi-episode marker.
	contLetterRe     = regexp.MustCompile(`(?i)^([ ._]*(?:[-&+~][ ._]*)?)(?:x?e|x|ep)[ ._]?(\d{1,4})`)
	contDashRe       = regexp.MustCompile(`^(-)(\d{1,4})`)
	contFullRe       = regexp.MustCompile(`(?i)^[ ._]*[-&+][ ._]*s?(\d{1,4})(?:x?e|x)(\d{1,4})`)
	sonarrAbsoluteRe = regexp.MustCompile(`^(\d{3,4})(?: - (.*))?$`)
	yearRangeRe      = regexp.MustCompile(`[\(\[]\s*((?:19|20)\d{2})\s*-\s*(?:(?:19|20)\d{2})?\s*[\)\]]`)
)

// epMatch is the raw result of one numbering matcher.
type epMatch struct {
	season   int // -1 unknown
	episodes []int
	airDate  string
	absolute int
	start    int // where the numbering starts (series title before)
	end      int // where it ends (episode title after)
}

// ParseEpisode parses an episode file path. It returns false when no
// episode numbering (SxxEyy, NxNN, air date, absolute number...) is found.
// The series title comes from the file name when present, otherwise from
// the first parent folder that is not a season folder. The season falls
// back to the parent season folder when the file name has none.
func ParseEpisode(path string) (Episode, bool) {
	ep := Episode{Season: -1}
	comps := splitPath(path)
	if len(comps) == 0 {
		return ep, false
	}
	raw := stripKnownExt(comps[len(comps)-1])
	ep.IDs, raw = extractIDs(raw)
	_, raw = extractEditionTag(raw)
	s := normalizeSeparators(raw)

	parentSeason, inSeason := -1, false
	if len(comps) >= 2 {
		parentSeason, inSeason = ParseSeasonFolder(comps[len(comps)-2])
	}

	m, ok := matchEpisode(s, inSeason)
	if !ok {
		return ep, false
	}
	ep.Season = m.season
	if ep.Season < 0 && inSeason {
		ep.Season = parentSeason
	}
	ep.Episodes = m.episodes
	ep.AirDate = m.airDate
	ep.Absolute = m.absolute

	if m.start > 0 {
		pi := parseName(s[:m.start])
		ep.SeriesTitle, ep.Year = pi.title, pi.year
	}
	if m.end < len(s) {
		rest := s[m.end:]
		if m.airDate != "" {
			if loc := timeRe.FindStringIndex(rest); loc != nil {
				rest = rest[loc[1]:]
			}
		}
		ep.Title = cleanTitle(rest, true)
		// Sonarr anime format: "Show - S01E01 - 001 - Title".
		if len(ep.Episodes) == 1 && ep.Absolute == 0 && ep.Season >= 0 {
			if am := sonarrAbsoluteRe.FindStringSubmatch(ep.Title); am != nil {
				ep.Absolute = atoi(am[1])
				ep.Title = am[2]
			}
		}
	}

	// Series folder: first parent that is not a season / extras folder.
	for i := len(comps) - 2; i >= 0 && i >= len(comps)-4; i-- {
		c := comps[i]
		if _, ok := ParseSeasonFolder(c); ok {
			continue
		}
		if extraFolders[normalizeFolderKey(c)] != ExtraNone || strings.HasSuffix(c, ":") {
			continue
		}
		sf := ParseSeriesFolder(c)
		if ep.SeriesTitle == "" {
			ep.SeriesTitle = sf.Title
		}
		if ep.Year == 0 {
			ep.Year = sf.Year
		}
		ep.IDs.merge(sf.IDs)
		break
	}
	return ep, true
}

// matchEpisode tries the numbering patterns from the most to the least
// specific one.
func matchEpisode(s string, inSeason bool) (epMatch, bool) {
	if m, ok := matchSE(s); ok {
		return m, true
	}
	if loc := wordySERe.FindStringSubmatchIndex(s); loc != nil {
		season, _ := strconv.Atoi(s[loc[4]:loc[5]])
		e, _ := strconv.Atoi(s[loc[6]:loc[7]])
		return epMatch{season: season, episodes: []int{e}, start: loc[2], end: loc[3]}, true
	}
	if m, ok := matchDate(s); ok {
		return m, true
	}
	if m, ok := matchNx(s); ok {
		return m, true
	}
	if m, ok := matchEpOnly(s); ok {
		return m, true
	}
	if ymdRe.MatchString(s) || dmyRe.MatchString(s) {
		return epMatch{}, false // an invalid or ambiguous date, not an absolute number
	}
	if inSeason || digitsRe.MatchString(s) || leadingRangeOnly(s) {
		if loc := leadingNumRe.FindStringSubmatchIndex(s); loc != nil {
			a, _ := strconv.Atoi(s[loc[4]:loc[5]])
			eps := []int{a}
			if loc[6] >= 0 {
				b, _ := strconv.Atoi(s[loc[6]:loc[7]])
				eps = expandRange(a, b)
			}
			return epMatch{season: -1, episodes: eps, start: 0, end: loc[3]}, true
		}
	}
	if m, ok := matchAbsolute(s); ok {
		return m, true
	}
	return epMatch{}, false
}

var leadingRangeRe = regexp.MustCompile(`^\d{1,3}-\d{1,3}$`)

func leadingRangeOnly(s string) bool { return leadingRangeRe.MatchString(s) }

func expandRange(a, b int) []int {
	if b <= a || b-a > maxRangeWidth {
		if b > a {
			return []int{a, b}
		}
		return []int{a}
	}
	out := make([]int, 0, b-a+1)
	for i := a; i <= b; i++ {
		out = append(out, i)
	}
	return out
}

func matchSE(s string) (epMatch, bool) {
	for _, loc := range seRe.FindAllStringSubmatchIndex(s, -1) {
		end := loc[7]
		if end < len(s) && s[end] >= '0' && s[end] <= '9' {
			continue
		}
		season, _ := strconv.Atoi(s[loc[4]:loc[5]])
		if season > 2100 {
			continue
		}
		first, _ := strconv.Atoi(s[loc[6]:loc[7]])
		eps, end := continueEpisodes(s, end, season, first)
		return epMatch{season: season, episodes: eps, start: loc[2], end: end}, true
	}
	return epMatch{}, false
}

func matchNx(s string) (epMatch, bool) {
	for _, loc := range nxRe.FindAllStringSubmatchIndex(s, -1) {
		season, _ := strconv.Atoi(s[loc[2]:loc[3]])
		first, _ := strconv.Atoi(s[loc[4]:loc[5]])
		eps, end := continueEpisodes(s, loc[5], season, first)
		return epMatch{season: season, episodes: eps, start: loc[2], end: end}, true
	}
	return epMatch{}, false
}

func matchEpOnly(s string) (epMatch, bool) {
	for _, loc := range epOnlyRe.FindAllStringSubmatchIndex(s, -1) {
		word := strings.ToLower(s[loc[2]:loc[4]])
		first, _ := strconv.Atoi(s[loc[4]:loc[5]])
		// A lone "e" must touch its number: "E07", not "e 7".
		if strings.TrimRight(word, " ._-") == "e" && len(word) > 1 {
			continue
		}
		eps, end := continueEpisodes(s, loc[5], -1, first)
		return epMatch{season: -1, episodes: eps, start: loc[2], end: end}, true
	}
	return epMatch{}, false
}

// continueEpisodes reads multi-episode continuations after the first episode
// number ending at pos: "E02", "-E03", "-03", "x04", " - 1x05", "&E06".
// Two numbers joined by a dash form a range that is expanded.
func continueEpisodes(s string, pos, season, first int) ([]int, int) {
	eps := []int{first}
	dashRange := false
	for len(eps) < maxEpisodes {
		rest := s[pos:]
		var n, adv int
		dash := false
		if m := contLetterRe.FindStringSubmatchIndex(rest); m != nil {
			sep := rest[m[2]:m[3]]
			// a plain space before the letter is a title ("S01E01 x264", "S01E01 E2 Title")
			if strings.TrimLeft(sep, " ._") == "" && sep != "" && strings.ContainsAny(sep, " ") {
				break
			}
			if strings.ContainsRune(sep, '.') && !strings.ContainsAny(sep, "-&+~") {
				break
			}
			n, _ = strconv.Atoi(rest[m[4]:m[5]])
			adv = m[1]
			dash = strings.ContainsAny(sep, "-~")
		} else if m := contFullRe.FindStringSubmatchIndex(rest); m != nil {
			sn, _ := strconv.Atoi(rest[m[2]:m[3]])
			if sn != season {
				break
			}
			n, _ = strconv.Atoi(rest[m[4]:m[5]])
			adv = m[1]
			dash = strings.Contains(rest[:m[2]], "-")
		} else if m := contDashRe.FindStringSubmatchIndex(rest); m != nil {
			n, _ = strconv.Atoi(rest[m[4]:m[5]])
			adv = m[1]
			dash = true
		} else {
			break
		}
		// the number must end the token: reject "-1080p", "x264", "E02.5"
		if pos+adv < len(s) {
			c := rune(s[pos+adv])
			if unicode.IsDigit(c) || c == 'p' || c == 'P' || c == 'i' || c == 'I' {
				break
			}
		}
		if n <= eps[len(eps)-1] || n == 264 || n == 265 {
			break
		}
		if len(eps) == 1 && dash {
			dashRange = true
		} else {
			dashRange = false
		}
		eps = append(eps, n)
		pos += adv
	}
	if dashRange && len(eps) == 2 {
		eps = expandRange(eps[0], eps[1])
	}
	return eps, pos
}

func validDate(y, mo, d int) bool {
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	return t.Year() == y && int(t.Month()) == mo && t.Day() == d
}

func matchDate(s string) (epMatch, bool) {
	for _, loc := range ymdRe.FindAllStringSubmatchIndex(s, -1) {
		y, _ := strconv.Atoi(s[loc[2]:loc[3]])
		mo, _ := strconv.Atoi(s[loc[6]:loc[7]])
		d, _ := strconv.Atoi(s[loc[8]:loc[9]])
		if !validDate(y, mo, d) {
			continue
		}
		return epMatch{season: -1, airDate: formatDate(y, mo, d), start: loc[2], end: loc[9]}, true
	}
	for _, loc := range dmyRe.FindAllStringSubmatchIndex(s, -1) {
		a, _ := strconv.Atoi(s[loc[4]:loc[5]])
		b, _ := strconv.Atoi(s[loc[6]:loc[7]])
		y, _ := strconv.Atoi(s[loc[8]:loc[9]])
		var mo, d int
		switch {
		case a == b:
			d, mo = a, b
		case a > 12 && b <= 12:
			d, mo = a, b
		case b > 12 && a <= 12:
			mo, d = a, b
		default:
			continue // ambiguous: 03.04.2024
		}
		if !validDate(y, mo, d) {
			continue
		}
		return epMatch{season: -1, airDate: formatDate(y, mo, d), start: loc[2], end: loc[3]}, true
	}
	return epMatch{}, false
}

func formatDate(y, mo, d int) string {
	return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func isYear(n int) bool { return n >= 1900 && n <= 2100 }

// matchAbsolute handles anime-style absolute numbering.
func matchAbsolute(s string) (epMatch, bool) {
	toks := tokenize(s)
	ts, _ := leadingGroups(s, toks)
	body := 0
	if ts < len(toks) {
		body = toks[ts].start
	} else {
		body = len(s)
	}
	for _, loc := range dashNumRe.FindAllStringSubmatchIndex(s, -1) {
		if loc[0] < body {
			continue
		}
		n, _ := strconv.Atoi(s[loc[4]:loc[5]])
		if isYear(n) && loc[5]-loc[4] == 4 {
			continue
		}
		return epMatch{season: -1, episodes: []int{n}, absolute: n, start: loc[0], end: loc[3]}, true
	}
	for _, loc := range bracketNumRe.FindAllStringSubmatchIndex(s, -1) {
		if loc[0] == 0 {
			continue // "[01] Title" style release group / index
		}
		n, _ := strconv.Atoi(s[loc[4]:loc[5]])
		if isYear(n) && loc[5]-loc[4] == 4 {
			continue
		}
		return epMatch{season: -1, episodes: []int{n}, absolute: n, start: loc[0], end: loc[1]}, true
	}
	if loc := trailingNumRe.FindStringSubmatchIndex(s); loc != nil && loc[3] > body {
		n, _ := strconv.Atoi(s[loc[6]:loc[7]])
		if !isYear(n) || loc[7]-loc[6] != 4 {
			prefix := s[:loc[3]]
			// "Show 1080p" / "x264": the trailing number must not be junk.
			if !isJunk(lastWord(prefix) + s[loc[3]:loc[5]]) {
				return epMatch{season: -1, episodes: []int{n}, absolute: n, start: loc[4], end: loc[5]}, true
			}
		}
	}
	return epMatch{}, false
}

func lastWord(s string) string {
	if i := strings.LastIndexAny(s, " -"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// ParseSeriesFolder parses a series folder name such as
// "Breaking Bad (2008) {tvdb-81189}" or "Case Closed (1996-2007)".
func ParseSeriesFolder(name string) Series {
	name = baseName(name)
	var sr Series
	sr.IDs, name = extractIDs(name)
	name = yearRangeRe.ReplaceAllString(name, "($1)")
	info := parseName(name)
	sr.Title, sr.Year = info.title, info.year
	return sr
}

var (
	seasonWordRe  = regexp.MustCompile(`(?i)^(?:season|saison|staffel|stagione|temporada|sæson|säsong|seizoen|seasong|sezon|sezona|sezóna|sezonul|kausi|series|сезон|시즌|シーズン)[ ._-]*(\d{1,4})((?:[ ._\-(\[].*)?)$`)
	seasonShortRe = regexp.MustCompile(`(?i)^s(\d{1,4})((?:[ ._\-(\[].*)?)$`)
	seasonNumRe   = regexp.MustCompile(`^\d{1,3}$`)
	specialsRe    = regexp.MustCompile(`(?i)^(?:specials?|spéciaux|speciaux|speciali|especiales|special episodes|season specials)$`)
	// release packs: "The.Wonder.Years.S04.PDTV.x264-JCH", "Breaking.Bad.Season.2.1080p"
	packSRe       = regexp.MustCompile(`(?i)(?:^|\s)s(\d{1,2})(?:\s|$)`)
	packSeasonRe  = regexp.MustCompile(`(?i)(?:^|\s)(?:season|saison|staffel|temporada)\s(\d{1,3})(?:\s|$)`)
	trailerYearRe = regexp.MustCompile(`^\s*[\(\[]\s*(19|20)\d{2}\s*[\)\]]\s*$`)
)

// ParseSeasonFolder reports the season number of a season folder name:
// "Season 1", "Saison 02", "Staffel 4", "Temporada 3", "S03", "Season 2024",
// "3", "Specials" (0), and release packs such as
// "Show.Name.S02.1080p.BluRay.x264-GRP". Names that merely contain the
// word, like "Season of the Witch" or "Seinfeld Season 2", are rejected.
func ParseSeasonFolder(name string) (season int, ok bool) {
	name = strings.TrimSpace(baseName(name))
	if name == "" {
		return 0, false
	}
	if specialsRe.MatchString(name) {
		return 0, true
	}
	n := strings.TrimSpace(normalizeSeparators(name))
	if m := seasonWordRe.FindStringSubmatch(n); m != nil {
		return seasonNumber(m[1], m[2])
	}
	if m := seasonShortRe.FindStringSubmatch(n); m != nil {
		return seasonNumber(m[1], m[2])
	}
	if seasonNumRe.MatchString(n) {
		v, _ := strconv.Atoi(n)
		return v, true
	}
	if seRe.MatchString(n) {
		return 0, false
	}
	for _, re := range []*regexp.Regexp{packSRe, packSeasonRe} {
		loc := re.FindStringSubmatchIndex(n)
		if loc == nil || loc[0] == 0 && re == packSeasonRe {
			continue
		}
		rest := strings.Fields(n[loc[3]:])
		if re == packSeasonRe && len(rest) == 0 {
			continue // "Seinfeld Season 2" is ambiguous
		}
		if len(rest) == 0 || isJunk(rest[0]) || strings.HasPrefix(rest[0], "[") || strings.HasPrefix(rest[0], "(") {
			v, _ := strconv.Atoi(n[loc[2]:loc[3]])
			return v, true
		}
	}
	return 0, false
}

// seasonNumber validates the number and the trailing text of a season name.
func seasonNumber(num, rest string) (int, bool) {
	v, err := strconv.Atoi(num)
	if err != nil || v > 2100 {
		return 0, false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" || trailerYearRe.MatchString(rest) {
		return v, true
	}
	switch rest[0] {
	case '-', '(', '[', '.', '_':
		return v, true
	}
	// "Season 1 1080p BluRay", but not "S01E02" or "Season 1 Episode 2"
	f := strings.Fields(rest)
	if len(f) > 0 && isJunk(f[0]) {
		return v, true
	}
	return 0, false
}
