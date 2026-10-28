package httpx

import (
	"math"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	cases := map[string]string{
		"Le Fabuleux Destin d'Amélie Poulain": "le fabuleux destin d amelie poulain",
		"Amélie":                              "amelie",
		"  Æon Flux!! ":                       "aeon flux",
		"Fast & Furious":                      "fast and furious",
		"S.W.A.T.":                            "swat",
		"Spider-Man: No Way Home":             "spider man no way home",
		"Œdipe roi":                           "oedipe roi",
		"Straße":                              "strasse",
		"Café":                               "cafe", // combining acute
	}
	for in, want := range cases {
		if got := NormalizeTitle(in); got != want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTitleSimilarity(t *testing.T) {
	if s := TitleSimilarity("AMELIE", "Amélie"); s != 1 {
		t.Errorf("case/diacritics: %v", s)
	}
	if s := TitleSimilarity("Matrix, The", "The Matrix"); s != 1 {
		t.Errorf("token order: %v", s)
	}
	if s := TitleSimilarity("Amelie", "Amelia"); s < 0.8 || s >= 0.9 {
		t.Errorf("near miss: %v", s)
	}
	if s := TitleSimilarity("1917", "Avatar"); s > 0.2 {
		t.Errorf("unrelated: %v", s)
	}
	if s := TitleSimilarity("Amélie", "Le Fabuleux Destin d'Amélie Poulain"); math.Abs(s-(0.6+0.4/6)) > 1e-9 {
		t.Errorf("containment: %v", s)
	}
	if s := TitleSimilarity("", "x"); s != 0 {
		t.Errorf("empty: %v", s)
	}
}

func TestYearScoreAndWeighted(t *testing.T) {
	for _, tc := range []struct {
		want, got int
		s         float64
		ok        bool
	}{{2001, 2001, 1, true}, {2001, 2002, 0.8, true}, {2001, 2000, 0.8, true}, {2001, 2005, 0, true}, {0, 2001, 0, false}, {2001, 0, 0, false}} {
		s, ok := YearScore(tc.want, tc.got)
		if s != tc.s || ok != tc.ok {
			t.Errorf("YearScore(%d,%d) = %v,%v", tc.want, tc.got, s, ok)
		}
	}
	var w Weighted
	w.Add(0.7, 1)
	w.Add(0.05, 1)
	if math.Abs(w.Score()-1) > 1e-9 {
		t.Errorf("renormalized: %v", w.Score())
	}
	w.Add(0.25, 0)
	if math.Abs(w.Score()-0.75) > 1e-9 {
		t.Errorf("with year miss: %v", w.Score())
	}
	if (&Weighted{}).Score() != 0 {
		t.Error("empty weighted")
	}
}
