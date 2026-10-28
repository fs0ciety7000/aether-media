package auth

import (
	"strconv"
	"strings"
)

// Ages offered by parental controls.
var ParentalAges = []int{0, 6, 10, 12, 16, 18}

var ratingAges = map[string]int{
	// France (CNC / CSA)
	"tp": 0, "tous publics": 0, "tous public": 0, "u": 0, "al": 0,
	// United States (MPA, TV)
	"g": 0, "pg": 10, "pg-13": 13, "r": 17, "nc-17": 18, "nr": -1, "unrated": -1, "not rated": -1,
	"tv-y": 0, "tv-y7": 7, "tv-y7-fv": 7, "tv-g": 0, "tv-pg": 10, "tv-14": 14, "tv-ma": 17,
	// United Kingdom (BBFC)
	"12a": 12, "15": 15, "r18": 18,
	// Germany (FSK) handled by the numeric rule; "fsk 12" etc. too.
}

// RatingAge maps an official rating ("12", "-16", "TP", "PG-13", "TV-MA",
// "FSK 16", "FR:12"...) to a minimum age. ok is false when unknown.
func RatingAge(rating string) (age int, ok bool) {
	r := strings.ToLower(strings.TrimSpace(rating))
	if i := strings.LastIndex(r, ":"); i >= 0 { // "FR:12"
		r = strings.TrimSpace(r[i+1:])
	}
	r = strings.TrimPrefix(r, "rated ")
	r = strings.TrimPrefix(r, "fsk")
	r = strings.TrimSpace(r)
	if r == "" {
		return 0, false
	}
	if a, found := ratingAges[r]; found {
		return a, a >= 0
	}
	// Numeric forms: "12", "-12", "12+", "16 ans", "ab 16".
	num := strings.TrimLeft(r, "-+ ab")
	end := 0
	for end < len(num) && num[end] >= '0' && num[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(num[:end])
	if err != nil || n > 21 {
		return 0, false
	}
	return n, true
}
