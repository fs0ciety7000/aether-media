package vfs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
	"github.com/fs0ciety7000/aether-media/internal/vfs/local"
)

func TestCleanRel(t *testing.T) {
	ok := map[string]string{
		"": "", ".": "", "a": "a", "a/b": "a/b", "a//b/": "a/b", "./a/./b": "a/b",
		"Films/Heat (1995)/heat.mkv": "Films/Heat (1995)/heat.mkv", "a..b": "a..b",
	}
	for in, want := range ok {
		got, err := vfs.CleanRel(in)
		if err != nil || got != want {
			t.Errorf("CleanRel(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"..", "../a", "a/../../b", "a/..", "/etc/passwd", "a\x00b", `a\..\b`} {
		if _, err := vfs.CleanRel(in); !errors.Is(err, vfs.ErrPath) {
			t.Errorf("CleanRel(%q) err = %v; want ErrPath", in, err)
		}
	}
}

func TestJoin(t *testing.T) {
	cases := [][3]string{{"", "", ""}, {"r", "", "r"}, {"", "a", "a"}, {"r", "a/b", "r/a/b"}, {"/r/", "a", "/r/a"}}
	for _, c := range cases {
		if got := vfs.Join(c[0], c[1]); got != c[2] {
			t.Errorf("Join(%q,%q) = %q; want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestReadFileLimit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.nfo"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	ctx := context.Background()
	b, err := vfs.ReadFileLimit(ctx, fsys, "a.nfo", 5)
	if err != nil || string(b) != "hello" {
		t.Fatalf("got %q, %v", b, err)
	}
	if _, err := vfs.ReadFileLimit(ctx, fsys, "a.nfo", 4); !errors.Is(err, vfs.ErrTooLarge) {
		t.Fatalf("err = %v; want ErrTooLarge", err)
	}
	if _, err := vfs.ReadFileLimit(ctx, fsys, "missing", 4); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("err = %v; want ErrNotExist", err)
	}
}
