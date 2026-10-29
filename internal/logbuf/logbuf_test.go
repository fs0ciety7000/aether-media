package logbuf

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestRingAndFilter(t *testing.T) {
	b := New(3)
	log := slog.New(b.Handler(slog.LevelDebug))
	log.Debug("un")
	log.Info("deux", "n", 2)
	log.Warn("trois")
	log.Error("quatre", "err", errors.New("boum"))
	all, total := b.Entries(Filter{})
	if total != 3 || len(all) != 3 || all[0].Msg != "deux" || all[2].Msg != "quatre" {
		t.Fatalf("ring = %+v (%d)", all, total)
	}
	if all[2].Attrs[0].Value != "boum" || all[2].Level != "ERROR" {
		t.Fatalf("attrs = %+v", all[2])
	}
	if got, _ := b.Entries(Filter{MinLevel: slog.LevelWarn}); len(got) != 2 {
		t.Fatalf("level filter: %+v", got)
	}
	if got, _ := b.Entries(Filter{Query: "BOUM"}); len(got) != 1 || got[0].Msg != "quatre" {
		t.Fatalf("query filter: %+v", got)
	}
	if got, _ := b.Entries(Filter{After: all[1].Seq}); len(got) != 1 {
		t.Fatalf("after: %+v", got)
	}
	if got, _ := b.Entries(Filter{Limit: 1}); len(got) != 1 || got[0].Msg != "quatre" {
		t.Fatalf("limit: %+v", got)
	}
}

func TestGroupsAndSubscribe(t *testing.T) {
	b := New(10)
	ch, stop := b.Subscribe()
	defer stop()
	slog.New(b.Handler(nil)).With("component", "stream").WithGroup("req").Info("ok", "path", "/x", slog.Group("g", "a", 1))
	e := <-ch
	got := Format(e)
	for _, want := range []string{"INFO", "ok", "component=stream", "req.path=/x", "req.g.a=1"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
}

func TestRedaction(t *testing.T) {
	b := New(10)
	log := slog.New(b.Handler(slog.LevelDebug))
	secrets := []string{"abcdef0123456789", "s3cr3t-value", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJl", "deadbeefcafe"}
	log.Info("GET https://api.themoviedb.org/3/movie/1?api_key=abcdef0123456789&language=fr")
	log.Info("requête", "token", "s3cr3t-value", "clientSecret", "s3cr3t-value", "Authorization", "Bearer s3cr3t-value")
	log.Warn("erreur", "err", errors.New(`refusé : {"access_token":"s3cr3t-value"}`), "jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJl")
	log.Debug("ffmpeg", "args", "-i http://127.0.0.1:8096/internal/vfs/3/1767225600/deadbeefcafe/Films/a.mkv")
	log.Info("sftp", "url", "sftp://bob:s3cr3t-value@seedbox:22/")
	entries, _ := b.Entries(Filter{MinLevel: slog.LevelDebug})
	for _, e := range entries {
		line := Format(e)
		for _, s := range secrets {
			if strings.Contains(line, s) {
				t.Errorf("secret %q leaked: %s", s, line)
			}
		}
	}
	if !strings.Contains(Format(entries[0]), "language=fr") || !strings.Contains(Format(entries[3]), "/internal/vfs/3/1767225600/[masqué]/Films/a.mkv") {
		t.Errorf("redaction too wide: %s / %s", Format(entries[0]), Format(entries[3]))
	}
}

// The signed proxy URL keeps its path intact: only the signature segment is
// masked, once, and masking twice changes nothing (a doubled folder name in
// a log line therefore comes from the path itself, not from the masking).
func TestRedactProxyURL(t *testing.T) {
	path := "21%20Grams%20%282003%29%20%7Btmdb-470%7D/21%20Grams%20%282003%29%20%5Btmdbid-470%5D%20-%20%5BMULTi%5D%5BBluray-1080p%5D%5BDTS%205.1%5D%5Bx264%5D-AiRLiNE.mkv"
	for _, sig := range []string{"OShlTd-LIQAbH7SSZBsFvib01FrfJTrMKPY6CvMsBok", "1234abcd_-", "-x"} {
		in := "http://127.0.0.1:8096/internal/vfs/1/1793302954/" + sig + "/" + path
		want := "http://127.0.0.1:8096/internal/vfs/1/1793302954/[masqué]/" + path
		got := Redact(in)
		if got != want {
			t.Fatalf("Redact(%q)\n= %q\nwant %q", in, got, want)
		}
		if again := Redact(got); again != want {
			t.Fatalf("Redact is not idempotent: %q", again)
		}
	}
}
