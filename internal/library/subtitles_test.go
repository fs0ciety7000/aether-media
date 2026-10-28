package library

import (
	"path/filepath"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

func TestFindSidecars(t *testing.T) {
	dir := t.TempDir()
	var entries []vfs.Entry
	for _, n := range []string{
		"Heat (1995).mkv", "Heat (1995).srt", "Heat (1995).fr.srt", "Heat (1995).fre.forced.srt",
		"Heat (1995).en.sdh.ass", "Heat (1995).director commentary.vtt", "Other.fr.srt", "Heat (1995).nfo",
	} {
		entries = append(entries, vfs.Entry{Path: n})
	}
	got := findSidecars(filepath.Join(dir, "Heat (1995).mkv"), entries)
	if len(got) != 5 {
		t.Fatalf("got %d sidecars: %+v", len(got), got)
	}
	by := map[string]sidecar{}
	for _, s := range got {
		by[filepath.Base(s.path)] = s
	}
	if s := by["Heat (1995).fre.forced.srt"]; s.language != "fre" || !s.forced || s.codec != "subrip" {
		t.Errorf("forced: %+v", s)
	}
	if s := by["Heat (1995).en.sdh.ass"]; s.language != "eng" || !s.hearingImpaired || s.codec != "ass" {
		t.Errorf("sdh: %+v", s)
	}
	if s := by["Heat (1995).fr.srt"]; s.language != "fre" {
		t.Errorf("fr: %+v", s)
	}
	if s := by["Heat (1995).director commentary.vtt"]; s.language != "" || s.title != "director commentary" {
		t.Errorf("titled: %+v", s)
	}
}
