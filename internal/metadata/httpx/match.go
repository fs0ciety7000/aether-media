package httpx

import (
	"sort"
	"strings"
	"unicode"
)

// Title matching helpers shared by remote providers to score search hits.

// foldMap covers the Latin letters whose decomposition is not a simple
// base letter + combining mark (stdlib has no NFD).
var foldMap = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'æ': "ae", 'ç': "c", 'ć': "c", 'č': "c", 'ď': "d", 'đ': "d", 'ð': "d",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ī': "i", 'į': "i", 'ı': "i",
	'ł': "l", 'ľ': "l", 'ñ': "n", 'ń': "n", 'ň': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ō': "o", 'ő': "o", 'œ': "oe",
	'ř': "r", 'ś': "s", 'š': "s", 'ş': "s", 'ß': "ss", 'ť': "t", 'ţ': "t", 'þ': "th",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ū': "u", 'ů': "u", 'ű': "u", 'ų': "u",
	'ý': "y", 'ÿ': "y", 'ź': "z", 'ż': "z", 'ž': "z",
}

// NormalizeTitle lower-cases s, strips diacritics, turns "&" into "and" and
// every other punctuation run into a single space.
func NormalizeTitle(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == '.':
			// "S.W.A.T." keeps its letters together.
			continue
		case r == '&':
			if !space {
				b.WriteByte(' ')
			}
			b.WriteString("and ")
			space = true
			continue
		}
		if f, ok := foldMap[r]; ok {
			b.WriteString(f)
			space = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// TitleSimilarity returns 0..1: the best of the Levenshtein ratio over the
// normalized strings, over their sorted tokens (word order insensitive),
// and a containment score when one title's words appear in sequence inside
// the other ("Amélie" in "Le Fabuleux Destin d'Amélie Poulain"):
// 0.6 + 0.4 × shorter/longer word counts.
func TitleSimilarity(a, b string) float64 {
	a, b = NormalizeTitle(a), NormalizeTitle(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	return max(levRatio(a, b), levRatio(sortTokens(a), sortTokens(b)), containment(a, b))
}

func containment(a, b string) float64 {
	ta, tb := strings.Fields(a), strings.Fields(b)
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	if len(ta) == 0 || len(ta) == len(tb) {
		return 0
	}
	for i := 0; i+len(ta) <= len(tb); i++ {
		match := true
		for j := range ta {
			if tb[i+j] != ta[j] {
				match = false
				break
			}
		}
		if match {
			return 0.6 + 0.4*float64(len(ta))/float64(len(tb))
		}
	}
	return 0
}

func sortTokens(s string) string {
	f := strings.Fields(s)
	sort.Strings(f)
	return strings.Join(f, " ")
}

func levRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	n := max(len(ra), len(rb))
	if n == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ra, rb))/float64(n)
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// YearScore is 1 for an exact match, 0.8 for ±1, else 0. ok is false when
// either year is unknown (the component should then be left out).
func YearScore(want, got int) (score float64, ok bool) {
	if want <= 0 || got <= 0 {
		return 0, false
	}
	switch want - got {
	case 0:
		return 1, true
	case 1, -1:
		return 0.8, true
	}
	return 0, true
}

// Weighted is a weighted mean over the components that are known; unknown
// components drop out and the remaining weights are renormalized, so a
// missing year is neutral.
type Weighted struct{ sum, w float64 }

// Add includes value v with weight w.
func (s *Weighted) Add(w, v float64) { s.sum += w * v; s.w += w }

// Score returns the mean clamped to 0..1.
func (s *Weighted) Score() float64 {
	if s.w == 0 {
		return 0
	}
	return min(1, max(0, s.sum/s.w))
}
