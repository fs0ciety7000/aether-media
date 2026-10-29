package sources

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/agent"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// Real Radarr names (spaces, parentheses, braces, brackets) on an agent whose
// share is the Movies folder: the walk, the stored location, the signed
// proxy URL given to FFmpeg and its decoding by the proxy must all keep the
// path intact, and the file must read back (regression: a doubled folder
// name "X (2003) {tmdb-470}X (2003) {tmdb-470}/…" was reported in a log).
func TestRadarrNamesThroughAgentAndProxy(t *testing.T) {
	names := []string{
		"21 Grams (2003) {tmdb-470}/21 Grams (2003) [tmdbid-470] - [MULTi][Bluray-1080p][DTS 5.1][x264]-AiRLiNE.mkv",
		"300 (2007) {tmdb-1271}/300 (2007) [tmdbid-1271] - [MULTi VFF][Bluray-1080p][TrueHD Atmos 7.1][x264]-FoX.mkv",
		"Amélie (2001) {tmdb-194}/Le Fabuleux Destin d'Amélie Poulain (2001) [tmdbid-194] - 100% [x265]#1.mkv",
	}
	movies := filepath.Join(t.TempDir(), "media", "Movies")
	content := map[string][]byte{}
	for i, n := range names {
		p := filepath.Join(movies, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		b := bytes.Repeat([]byte{byte('a' + i)}, 100_000+i)
		content[n] = b
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, _ := newTestManager(t)
	m.ProxyBase = "http://127.0.0.1:8096"
	a, _ := startCountedAgent(t, 0, func(c *agent.Config) { c.Shares = []agent.Share{{Name: "films", Path: movies}} })
	_, src := pairedSource(t, m, a, "https")
	ctx := context.Background()

	fsys, _, err := m.FS(ctx, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	var walked []string
	if err := fsys.Walk(ctx, "", func(e vfs.Entry) error {
		if !e.IsDir {
			walked = append(walked, e.Path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(walked) != len(names) {
		t.Fatalf("walked %q", walked)
	}
	for _, rel := range walked {
		want, ok := content[rel]
		if !ok {
			t.Fatalf("walk returned %q, not one of the files", rel)
		}
		loc := Location(src, rel)
		if loc != "src:"+strconv.FormatInt(src.ID, 10)+":/"+rel {
			t.Fatalf("location %q", loc)
		}
		in, err := m.InputURL(ctx, loc)
		if err != nil {
			t.Fatal(err)
		}
		// What the server's mux gives the proxy handler: the decoded path
		// after /internal/vfs/{id}/{exp}/{sig}/.
		u, err := url.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/internal/vfs/"), "/", 4)
		if len(parts) != 4 || parts[3] != rel {
			t.Fatalf("proxy URL %q decodes to %q, want %q", in, parts, rel)
		}
		id, _ := strconv.ParseInt(parts[0], 10, 64)
		exp, _ := strconv.ParseInt(parts[1], 10, 64)
		got, ok := m.VerifyProxy(id, exp, parts[2], parts[3])
		if !ok || got != loc {
			t.Fatalf("VerifyProxy = %q, %v; want %q", got, ok, loc)
		}
		f, err := m.Open(ctx, got, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil || !bytes.Equal(b, want) {
			t.Fatalf("read %s: %d bytes, %v", rel, len(b), err)
		}
	}
}
