package auth

import (
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword(h, "correct horse"); !ok || err != nil {
		t.Fatalf("verify: %v %v", ok, err)
	}
	if ok, _ := VerifyPassword(h, "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse")
	if h == h2 {
		t.Fatal("salt not random")
	}
	if _, err := VerifyPassword("$argon2id$garbage", "x"); err == nil {
		t.Fatal("malformed hash accepted")
	}
}

func TestTokens(t *testing.T) {
	tok, hash, err := NewToken()
	if err != nil || len(tok) < 40 || string(HashToken(tok)) != string(hash) {
		t.Fatalf("token %q", tok)
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }
	for range 3 {
		l.Fail("ip")
	}
	if !l.Blocked("ip") || l.Blocked("other") {
		t.Fatal("limiter state")
	}
	now = now.Add(2 * time.Minute)
	if l.Blocked("ip") {
		t.Fatal("window not expired")
	}
	l.Fail("ip")
	l.Reset("ip")
	if l.Blocked("ip") {
		t.Fatal("reset")
	}
}

func TestRatingAge(t *testing.T) {
	cases := map[string]int{"TP": 0, "Tous publics": 0, "-12": 12, "12": 12, "16+": 16, "FR:-16": 16,
		"PG-13": 13, "R": 17, "TV-MA": 17, "TV-Y7": 7, "FSK 6": 6, "12A": 12, "Rated PG": 10}
	for in, want := range cases {
		if got, ok := RatingAge(in); !ok || got != want {
			t.Errorf("%q → %d %v, want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "NR", "Unrated", "XYZ", "99"} {
		if _, ok := RatingAge(in); ok {
			t.Errorf("%q should be unknown", in)
		}
	}
}
