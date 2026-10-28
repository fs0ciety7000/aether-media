package naming

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	hDotRe        = regexp.MustCompile(`(?i)\bh\.?(26[45])\b`)
	resolutionRe  = regexp.MustCompile(`(?i)^\d{3,4}[pi]$`)
	dimensionRe   = regexp.MustCompile(`(?i)^\d{3,4}x\d{3,4}[pi]?$`)
	audioChanRe   = regexp.MustCompile(`(?i)^(dd|ddp|dd\+|eac3|ac3|aac|dts|truehd|flac|opus|lpcm|pcm|mp3)\d(\.\d)?$`)
	yearTokenRe   = regexp.MustCompile(`^(19\d{2}|20\d{2}|2100)$`)
	stackTokenRe  = regexp.MustCompile(`(?i)^(cd|dvd|disc|disk|pt)[-.]?(\d{1,2})$`)
	partTokenRe   = regexp.MustCompile(`(?i)^part[-.]?(\d{1,2})$`)
	bracketYearRe = regexp.MustCompile(`[\(\[]\s*(19\d{2}|20\d{2}|2100)\s*[\)\]]`)
	groupRe       = regexp.MustCompile(`\[[^\]]*\]|\{[^}]*\}`)
	spaceRe       = regexp.MustCompile(`\s+`)
	// "Dragon-HDC": an upper-case release group glued to a mixed-case word.
	groupSuffixRe = regexp.MustCompile(`(\p{L}*\p{Ll}\p{L}*)-[0-9]*[A-Z][A-Z0-9]{1,9}$`)
)

// strongJunk are release tokens matched case-insensitively. They always end
// a title (unless they are its very first word).
var strongJunk = set(
	// resolutions / quality
	"4k", "8k", "uhd", "ultrahd", "fhd", "qhd", "hdr", "hdr10", "hdr10+", "hdr10plus", "hlg", "dovi", "dolby", "sdr",
	// sources
	"bluray", "blu-ray", "bdrip", "brrip", "bdremux", "bd25", "bd50", "bdmv", "web-dl", "webdl", "webrip", "web-rip",
	"webcap", "hdtv", "pdtv", "sdtv", "dsr", "dsrip", "dvdrip", "dvdscr", "dvd", "dvd5", "dvd9", "dvdr", "hdrip",
	"hddvd", "remux", "hdcam", "hdts", "telesync", "telecine", "tvrip", "hdtvrip", "vhsrip", "uhdrip", "screener",
	"amzn", "dsnp", "hmax", "atvp", "pcok", "pmtp",
	// video codecs
	"x264", "x265", "h264", "h265", "hevc", "avc", "av1", "xvid", "divx", "vp9", "10bit", "10-bit", "8bit", "hi10p",
	"hi10", "mpeg2", "vc1", "vc-1",
	// audio
	"dts", "dts-hd", "dtshd", "dts-x", "dtsx", "dts-ma", "truehd", "atmos", "ac3", "eac3", "ddp", "dd+", "aac",
	"flac", "opus", "lpcm", "pcm", "mp3",
	// languages
	"multi", "multisub", "multisubs", "vff", "vfq", "vfi", "vf2", "vostfr", "vosta", "truefrench", "subfrench",
	// release flags
	"proper", "repack", "rerip", "internal", "readnfo", "nfofix", "dubbed", "subbed", "hardsub", "hardsubs", "stv",
	// 3D layouts
	"hsbs", "h-sbs", "half-sbs", "fsbs", "full-sbs", "htab", "h-tab", "half-tab", "ftab", "full-tab", "mvc",
)

// upperJunk are release tokens that are also ordinary words; they only count
// as junk when written in upper case, as scene releases do.
var upperJunk = set("WEB", "DL", "FRENCH", "GERMAN", "ITALIAN", "SPANISH", "ENGLISH", "DUTCH", "JAPANESE",
	"KOREAN", "DUAL", "CAM", "TS", "HD", "SD", "DV", "NF", "HULU", "VO", "VF", "VOST", "LIMITED", "REAL",
	"COMPLETE", "HC", "NTSC", "PAL", "WS", "SUBS", "FS")

// isJunk reports whether a single token is release junk.
func isJunk(tok string) bool {
	tok = strings.TrimLeft(tok, "-")
	if tok == "" {
		return false
	}
	if upperJunk[tok] {
		return true
	}
	l := strings.ToLower(tok)
	if strongJunk[l] || resolutionRe.MatchString(l) || dimensionRe.MatchString(l) || audioChanRe.MatchString(l) ||
		stackTokenRe.MatchString(l) || partTokenRe.MatchString(l) {
		return true
	}
	// "x264-GROUP", "WEBDL-1080p", "DTS-HD.MA": the first hyphen part decides.
	if i := strings.IndexByte(tok, '-'); i > 0 && i < len(tok)-1 {
		first := tok[:i]
		if upperJunk[first] {
			return true
		}
		fl := strings.ToLower(first)
		return strongJunk[fl] || resolutionRe.MatchString(fl) || audioChanRe.MatchString(fl)
	}
	return false
}

// editionPhrase is a sequence of lower-cased, apostrophe-free, diacritic-free
// words. Strong phrases may end a title anywhere; weak ones only count after
// the year or other junk.
type editionPhrase struct {
	words  []string
	strong bool
}

var editionPhrases = func() []editionPhrase {
	strong := []string{
		"directors cut", "director cut", "directors edition", "extended cut", "extended edition",
		"extended version", "theatrical cut", "theatrical edition", "theatrical version", "unrated cut",
		"unrated edition", "imax edition", "criterion collection", "special edition", "collectors edition",
		"anniversary edition", "ultimate edition", "limited edition", "version longue", "version integrale",
		"edition speciale", "version longue integrale", "montage du realisateur", "extended", "theatrical",
		"unrated", "uncut", "uncensored", "remastered", "imax", "criterion",
	}
	weak := []string{"final cut", "ultimate cut", "rogue cut", "open matte", "redux", "despecialized", "remaster"}
	var out []editionPhrase
	for _, p := range strong {
		out = append(out, editionPhrase{strings.Fields(p), true})
	}
	for _, p := range weak {
		out = append(out, editionPhrase{strings.Fields(p), false})
	}
	// Longest phrases first so "extended edition" wins over "extended".
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && len(out[j].words) > len(out[j-1].words); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}()

// token is a word of a normalized name with its byte offsets.
type token struct {
	text       string
	key        string // editionKey(text)
	start, end int
}

func (t token) isSep() bool {
	switch t.text {
	case "-", "–", "—", "~", "(", ")", "[", "]", "{", "}", ",", ":", ";", "+", "&":
		return true
	}
	return false
}

func (t token) isOpen() bool { return t.text == "(" || t.text == "[" || t.text == "{" }

// tokenize splits s on white space; brackets are tokens of their own.
func tokenize(s string) []token {
	var toks []token
	start := -1
	flush := func(i int) {
		if start >= 0 {
			toks = append(toks, token{s[start:i], editionKey(s[start:i]), start, i})
			start = -1
		}
	}
	for i, r := range s {
		switch {
		case unicode.IsSpace(r):
			flush(i)
		case strings.ContainsRune("()[]{}", r):
			flush(i)
			toks = append(toks, token{s[i : i+1], s[i : i+1], i, i + 1})
		default:
			if start < 0 {
				start = i
			}
		}
	}
	flush(len(s))
	return toks
}

// editionAt reports the edition phrase starting at toks[i], if any.
func editionAt(toks []token, i int) (n int, strong bool) {
	if !editionFirstWords[toks[i].key] {
		return 0, false
	}
	for _, p := range editionPhrases {
		if i+len(p.words) > len(toks) {
			continue
		}
		ok := true
		for k, w := range p.words {
			if toks[i+k].key != w {
				ok = false
				break
			}
		}
		if ok {
			return len(p.words), p.strong
		}
	}
	return 0, false
}

var (
	apostropheReplacer = strings.NewReplacer("'", "", "’", "")
	editionFirstWords  = func() map[string]bool {
		m := map[string]bool{}
		for _, p := range editionPhrases {
			m[p.words[0]] = true
		}
		return m
	}()
)

func editionKey(s string) string {
	s = apostropheReplacer.Replace(s)
	return foldDiacritics(strings.ToLower(s))
}

// stackAt reports a multi-part marker starting at toks[i]: "cd1", "disc 2",
// "pt3", "part1" (strong) or "part 2" (weak, only meaningful after a year).
func stackAt(toks []token, i int) (part, n int, strong bool) {
	t := strings.TrimLeft(toks[i].text, "-")
	if m := stackTokenRe.FindStringSubmatch(t); m != nil {
		p, _ := strconv.Atoi(m[2])
		return p, 1, true
	}
	if m := partTokenRe.FindStringSubmatch(t); m != nil {
		p, _ := strconv.Atoi(m[1])
		return p, 1, true
	}
	if i+1 < len(toks) && len(toks[i+1].text) <= 2 && digitsRe.MatchString(toks[i+1].text) {
		p, _ := strconv.Atoi(toks[i+1].text)
		switch strings.ToLower(t) {
		case "cd", "disc", "disk", "pt", "dvd":
			return p, 2, true
		case "part", "partie":
			return p, 2, false
		}
	}
	return 0, 0, false
}

func is3DToken(t string) bool {
	l := strings.ToLower(t)
	switch l {
	case "3d", "hsbs", "h-sbs", "half-sbs", "fsbs", "full-sbs", "sbs", "htab", "h-tab", "half-tab", "ftab",
		"full-tab", "hou", "h-ou", "half-ou", "mvc":
		return true
	}
	return false
}

// normalizeSeparators turns underscores and word-separating dots into spaces
// while keeping acronyms ("S.H.I.E.L.D.", "L.A."), common abbreviations
// ("Mr.", "St.", "Vol.") and audio channel layouts ("DD5.1").
func normalizeSeparators(s string) string {
	s = capLen(s)
	s = strings.ReplaceAll(s, "_", " ")
	if strings.ContainsRune(s, '.') {
		s = hDotRe.ReplaceAllString(s, "H$1")
	}
	fields := strings.Fields(s)
	for i, f := range fields {
		fields[i] = normalizeChunk(f)
	}
	return strings.Join(strings.Fields(strings.Join(fields, " ")), " ")
}

// maxNameLen bounds the length of a single name that is parsed; real file
// names are at most 255 bytes on common filesystems.
const maxNameLen = 1024

// capLen truncates s to maxNameLen bytes on a rune boundary.
func capLen(s string) string {
	if len(s) <= maxNameLen {
		return s
	}
	i := maxNameLen
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i]
}

var abbreviations = set("mr", "mrs", "ms", "dr", "st", "jr", "sr", "vs", "mt", "ft", "vol")

func isSingleLetter(p string) bool {
	r, n := utf8.DecodeRuneInString(p)
	return n > 0 && n == len(p) && unicode.IsLetter(r)
}

func hasLetter(p string) bool {
	return strings.IndexFunc(p, unicode.IsLetter) >= 0
}

func normalizeChunk(c string) string {
	if !strings.ContainsRune(c, '.') {
		return c
	}
	if strings.HasSuffix(c, "...") && len(c) > 3 {
		return normalizeChunk(strings.TrimRight(c, ".")) + "..."
	}
	parts := strings.Split(c, ".")
	n := len(parts)
	acr := make([]bool, n)
	for i := 0; i < n; {
		if !isSingleLetter(parts[i]) {
			i++
			continue
		}
		j := i
		for j < n && isSingleLetter(parts[j]) {
			j++
		}
		if j-i >= 2 {
			for k := i; k < j; k++ {
				acr[k] = true
			}
		}
		i = j
	}
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(p)
		if i == n-1 {
			break
		}
		next := parts[i+1]
		switch {
		case acr[i]:
			b.WriteByte('.')
			if !acr[i+1] && next != "" {
				b.WriteByte(' ')
			}
		case p != "" && abbreviations[strings.ToLower(p)]:
			b.WriteByte('.')
			if next != "" {
				b.WriteByte(' ')
			}
		case len(next) == 1 && next[0] >= '0' && next[0] <= '9' && p != "" &&
			p[len(p)-1] >= '0' && p[len(p)-1] <= '9' && (len(p) == 1 || hasLetter(p)):
			b.WriteByte('.')
		default:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// leadingGroups returns the index of the first token after the leading
// "[group]" blocks and separators, plus the content of the last such group.
func leadingGroups(s string, toks []token) (int, string) {
	i := 0
	last := ""
	for i < len(toks) {
		if toks[i].text == "[" {
			j := i + 1
			for j < len(toks) && toks[j].text != "]" {
				j++
			}
			if j == len(toks) {
				break
			}
			if j > i+1 {
				last = s[toks[i+1].start:toks[j-1].end]
			}
			i = j + 1
			continue
		}
		if toks[i].isSep() && !toks[i].isOpen() {
			i++
			continue
		}
		break
	}
	return i, last
}

// finalizeTitle removes leftover bracket groups and dangling separators.
func finalizeTitle(t string) string {
	if strings.ContainsAny(t, "[{") {
		if r := groupRe.ReplaceAllString(t, " "); strings.TrimSpace(r) != "" {
			t = r
		}
	}
	t = spaceRe.ReplaceAllString(t, " ")
	t = groupSuffixRe.ReplaceAllString(t, "$1")
	for {
		before := t
		t = strings.TrimSpace(t)
		t = strings.TrimRight(t, "-–—_,:;~+([{&")
		t = strings.TrimLeft(t, "-–—_,:;~+)]}&.")
		t = strings.TrimSpace(t)
		// drop an unbalanced trailing "(" group such as "Movie (Part"
		if i := strings.LastIndexByte(t, '('); i > 0 && !strings.ContainsRune(t[i:], ')') {
			t = t[:i]
		}
		if t == before {
			return t
		}
	}
}

// nameInfo is the result of parsing a release-style name.
type nameInfo struct {
	title   string
	year    int
	edition string
	part    int
	is3D    bool
	junk    bool // release junk was found after the title
}

// parseName parses a movie-like name: "Title (Year) [junk]" or
// "Title.Year.1080p.BluRay.x264-GROUP".
func parseName(raw string) nameInfo {
	var info nameInfo
	info.edition, raw = extractEditionTag(raw)
	_, raw = extractIDs(raw)
	s := normalizeSeparators(raw)
	toks := tokenize(s)
	if len(toks) == 0 {
		return info
	}
	ts, lastGroup := leadingGroups(s, toks)

	// Locate the year: last bracketed year, else the last plausible bare year
	// followed by junk or by the end of the name.
	yearIdx, yearEnd := -1, -1
	for _, loc := range bracketYearRe.FindAllStringSubmatchIndex(s, -1) {
		for i := range toks {
			if toks[i].start == loc[0] {
				j := i
				for j < len(toks) && toks[j].end < loc[1] {
					j++
				}
				if j < len(toks) {
					yearIdx, yearEnd = i, j+1
					info.year, _ = strconv.Atoi(s[loc[2]:loc[3]])
				}
				break
			}
		}
	}
	if yearIdx < 0 {
		follow := yearFollowTable(toks)
		for i := ts + 1; i < len(toks); i++ {
			if !yearTokenRe.MatchString(toks[i].text) {
				continue
			}
			if follow[i+1] {
				yearIdx, yearEnd = i, i+1
				info.year, _ = strconv.Atoi(toks[i].text)
			}
		}
	}

	te := len(toks)
	if yearIdx >= 0 {
		te = yearIdx
	}
	if yearIdx >= 0 && yearIdx <= ts {
		// "[2019] Movie" or "(1995) Heat": title follows the year.
		ts, te = yearEnd, len(toks)
		for ts < te && toks[ts].isSep() && !toks[ts].isOpen() {
			ts++
		}
	}
	cut := te
	for i := ts + 1; i < te; i++ {
		if isJunk(toks[i].text) {
			cut = i
			info.junk = true
			break
		}
		if n, strong := editionAt(toks, i); n > 0 && strong {
			cut = i
			break
		}
	}
	// Edition, stack and 3D markers after the title.
	for i := cut; i < len(toks); i++ {
		if i >= yearIdx && i < yearEnd {
			continue
		}
		if info.edition == "" {
			if n, strong := editionAt(toks, i); n > 0 && (strong || (i >= yearEnd && yearEnd > 0) || info.junk) {
				info.edition = s[toks[i].start:toks[i+n-1].end]
			} else if n := genericEditionAt(toks, i); n > 0 && yearEnd > 0 && i == yearEnd {
				info.edition = s[toks[i].start:toks[i+n-1].end]
			}
		}
		if info.part == 0 {
			if p, _, strong := stackAt(toks, i); p > 0 && (strong || (yearEnd > 0 && i >= yearEnd)) {
				info.part = p
			}
		}
		if i > cut && isJunk(toks[i].text) {
			info.junk = true
		}
	}
	for _, t := range toks {
		if is3DToken(t.text) {
			info.is3D = true
			break
		}
	}
	if ts < cut && ts < len(toks) {
		end := len(s)
		if cut < len(toks) {
			end = toks[cut].start
		}
		info.title = finalizeTitle(s[toks[ts].start:end])
	}
	if info.title == "" && lastGroup != "" {
		info.title = finalizeTitle(lastGroup)
	}
	return info
}

// genericEditionAt recognises free-form editions right after a year, such as
// "Black and Chrome Edition" or "Rogue One Cut": up to four words ending with
// "edition", "cut" or "version". It returns the number of tokens, or 0.
func genericEditionAt(toks []token, i int) int {
	for j := i; j < len(toks) && j < i+4; j++ {
		if toks[j].isSep() {
			return 0
		}
		switch toks[j].key {
		case "edition", "cut", "version":
			if j > i {
				return j - i + 1
			}
			return 0
		}
	}
	return 0
}

// yearFollowTable reports, for each token index i, whether the token
// stream from i on starts with release junk, an edition, a stack marker, a
// bracketed group or nothing: a year right before such a position is the
// release year.
func yearFollowTable(toks []token) []bool {
	f := make([]bool, len(toks)+1)
	f[len(toks)] = true
	for i := len(toks) - 1; i >= 0; i-- {
		t := toks[i]
		if t.isSep() {
			f[i] = t.text == "[" || t.text == "{" || f[i+1]
			continue
		}
		switch {
		case isJunk(t.text) || is3DToken(t.text):
			f[i] = true
		case yearTokenRe.MatchString(t.text):
			f[i] = f[i+1]
		default:
			if n, _ := editionAt(toks, i); n > 0 {
				f[i] = true
			} else if genericEditionAt(toks, i) > 0 {
				f[i] = true
			} else if p, _, _ := stackAt(toks, i); p > 0 {
				f[i] = true
			}
		}
	}
	return f
}

// CleanTitle turns a release-style name into a human title: dots and
// underscores become spaces (acronyms such as "S.H.I.E.L.D." and
// abbreviations such as "Mr." are kept), leading "[group]" tags, provider id
// tags and bracketed junk are removed, and everything from the first release
// token on (resolution, source, codec, audio, HDR, language, release flags,
// "-GROUP" suffix) is dropped. Years are kept; use ParseMovie to split them.
func CleanTitle(s string) string {
	return cleanTitle(s, false)
}

// cleanTitle implements CleanTitle. When dropLeadingJunk is set a name that
// starts with release junk yields "" (used for episode titles).
func cleanTitle(raw string, dropLeadingJunk bool) string {
	_, raw = extractEditionTag(raw)
	_, raw = extractIDs(raw)
	s := normalizeSeparators(raw)
	toks := tokenize(s)
	ts, lastGroup := leadingGroups(s, toks)
	for ts < len(toks) && toks[ts].isSep() && !toks[ts].isOpen() {
		ts++
	}
	if ts >= len(toks) {
		if dropLeadingJunk {
			return ""
		}
		return finalizeTitle(lastGroup)
	}
	if dropLeadingJunk && (isJunk(toks[ts].text) || (toks[ts].isOpen() && ts+1 < len(toks) && isJunk(toks[ts+1].text))) {
		return ""
	}
	cut := len(toks)
	for i := ts + 1; i < len(toks); i++ {
		if isJunk(toks[i].text) {
			cut = i
			break
		}
		if n, strong := editionAt(toks, i); n > 0 && strong {
			cut = i
			break
		}
	}
	end := len(s)
	if cut < len(toks) {
		end = toks[cut].start
	}
	t := finalizeTitle(s[toks[ts].start:end])
	if t == "" && !dropLeadingJunk {
		t = finalizeTitle(lastGroup)
	}
	return t
}

// diacritics maps lower-case accented letters to their ASCII base.
var diacritics = func() map[rune]string {
	m := map[rune]string{}
	for base, letters := range map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ďđð", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥħ",
		"i": "ìíîïĩīĭįı", "j": "ĵ", "k": "ķ", "l": "ĺļľŀł", "n": "ñńņňŉ", "o": "òóôõöøōŏő",
		"r": "ŕŗř", "s": "śŝşšș", "t": "ţťŧț", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž",
		"ae": "æ", "oe": "œ", "ss": "ß", "th": "þ",
	} {
		for _, r := range letters {
			m[r] = base
		}
	}
	return m
}()

// foldDiacritics replaces accented lower-case letters by their ASCII base.
func foldDiacritics(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if rep, ok := diacritics[r]; ok {
			b.WriteString(rep)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var sortArticles = []string{"the ", "a ", "an ", "le ", "la ", "les ", "l'", "un ", "une ", "der ", "die ",
	"das ", "el ", "los ", "las ", "il "}

// sortArticleExceptions are titles whose first word looks like an article
// but is not one.
var sortArticleExceptions = []string{"die hard", "die another day", "die hart", "a.i.", "a-ha"}

// SortTitle returns a case-insensitive sort key for title: lower-cased,
// diacritics removed, leading punctuation and a leading article (The, A, An,
// Le, La, Les, L', Un, Une, Der, Die, Das, El, Los, Las, Il) dropped. Numbers
// are kept as-is.
func SortTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, "’", "'")
	s = foldDiacritics(s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimLeftFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, ex := range sortArticleExceptions {
		if strings.HasPrefix(s, ex) {
			return s
		}
	}
	for _, a := range sortArticles {
		if strings.HasPrefix(s, a) && len(s) > len(a) {
			rest := strings.TrimLeftFunc(s[len(a):], func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
			if rest != "" {
				return rest
			}
		}
	}
	return s
}
