package naming

import (
	"regexp"
	"strings"
)

// Movie is the information parsed from a movie file path.
type Movie struct {
	Title   string
	Year    int    // 0 if unknown
	Edition string // "{edition-Director's Cut}" (Radarr/Plex) or recognised edition words, "" otherwise
	IDs     ProviderIDs
	Part    int // multi-part stack index (cd1, part2, disc 1, pt3, -a/-b); 0 = not stacked
	Extra   ExtraType
	Is3D    bool
}

var (
	// "Heat (1995)-a", "Heat [1995] - b": letter stacks only right after a closing bracket.
	letterStackRe = regexp.MustCompile(`(?i)[\)\]]\s*-\s*([a-d])$`)
	// "cd a", "part-b", "disc.c"
	keywordLetterStackRe = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:cd|dvd|part|pt|disc|disk)[\s._-]*([a-d])$`)
	stackFolderRe        = regexp.MustCompile(`(?i)^(?:cd|dvd|disc|disk|part)[\s._-]*(\d{1,2})$`)
	discImageNameRe      = regexp.MustCompile(`(?i)^(?:vts_\d+_\d+|video_ts|\d{5}|index|main|movie|film|video|feature|title_?t?\d*)$`)
)

// structural folders that never carry the movie name.
var skipFolders = set("video_ts", "bdmv", "stream", "playlist", "clipinf", "backup", "certificate", "audio_ts", "hvdvd_ts")

// ParseMovie parses a movie file path. The file name is parsed first; the
// parent movie folder (skipping extras, disc and structural folders) fills
// in what the file name lacks: "Movies/Heat (1995)/heat.mkv" gives Heat,
// 1995. Provider ids found in the movie folder name are merged in.
func ParseMovie(path string) Movie {
	comps := splitPath(path)
	var m Movie
	if len(comps) == 0 {
		return m
	}
	m.Extra = ClassifyExtra(path)
	name := stripKnownExt(comps[len(comps)-1])
	if m.Extra != ExtraNone {
		name = stripExtraSuffix(name)
	}
	m.IDs, _ = extractIDs(name)

	// Letter stacks ("-a", "cd b") are removed before the generic parse.
	if loc := letterStackRe.FindStringSubmatchIndex(name); loc != nil {
		m.Part = int(strings.ToLower(name[loc[2]:loc[3]])[0]-'a') + 1
		name = name[:loc[0]+1]
	} else if loc := keywordLetterStackRe.FindStringSubmatchIndex(name); loc != nil {
		m.Part = int(strings.ToLower(name[loc[2]:loc[3]])[0]-'a') + 1
		name = name[:loc[0]]
	}

	info := parseName(name)
	m.Title, m.Year, m.Edition, m.Is3D = info.title, info.year, info.edition, info.is3D
	if m.Part == 0 {
		m.Part = info.part
	}

	// Find the movie folder.
	var folder string
	for i := len(comps) - 2; i >= 0; i-- {
		c := comps[i]
		l := strings.ToLower(strings.TrimSpace(c))
		if sm := stackFolderRe.FindStringSubmatch(l); sm != nil {
			if m.Part == 0 {
				m.Part = atoi(sm[1])
			}
			continue
		}
		if skipFolders[l] || extraFolders[normalizeFolderKey(c)] != ExtraNone {
			continue
		}
		if strings.HasSuffix(c, ":") { // "C:" drive
			break
		}
		folder = c
		break
	}
	if folder == "" {
		return m
	}
	fids, _ := extractIDs(folder)
	f := parseName(folder)
	generic := m.Title == "" || discImageNameRe.MatchString(strings.TrimSpace(name)) ||
		(m.Part > 0 && strings.EqualFold(m.Title, "part"))
	switch {
	case generic:
		if f.title != "" {
			m.Title = f.title
		}
		if m.Year == 0 {
			m.Year = f.year
		}
		if m.Edition == "" {
			m.Edition = f.edition
		}
		m.Is3D = m.Is3D || f.is3D
		m.IDs.merge(fids)
	default:
		same := looseKey(f.title) == looseKey(m.Title)
		related := same || (f.title != "" && (strings.Contains(looseKey(f.title), looseKey(m.Title)) ||
			strings.Contains(looseKey(m.Title), looseKey(f.title))))
		if same && (f.year == m.Year || m.Year == 0) && strings.ToLower(m.Title) == m.Title {
			m.Title = f.title // better casing from the folder
		}
		if m.Year == 0 && related {
			m.Year = f.year
		}
		if related || f.year != 0 || !fids.IsZero() {
			m.IDs.merge(fids)
		}
		if m.Edition == "" && related {
			m.Edition = f.edition
		}
	}
	return m
}

// looseKey is a comparison key ignoring case, diacritics and punctuation.
func looseKey(s string) string {
	s = foldDiacritics(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
