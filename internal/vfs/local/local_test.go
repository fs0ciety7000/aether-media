package local

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

func write(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) (string, vfs.FS) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "root")
	write(t, filepath.Join(base, "secret.txt"), []byte("secret"))
	write(t, filepath.Join(root, "Films", "Heat (1995)", "heat.mkv"), bytes.Repeat([]byte("x"), 1000))
	write(t, filepath.Join(root, "Films", "Heat (1995)", "heat.nfo"), []byte("<movie/>"))
	write(t, filepath.Join(root, "Series", "S01", "e1.mkv"), []byte("e1"))
	write(t, filepath.Join(root, "a.txt"), []byte("a"))
	if err := os.Symlink(filepath.Join(base, "secret.txt"), filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(root, "escapedir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "inside.txt")); err != nil {
		t.Fatal(err)
	}
	f, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return root, f
}

func TestConfinement(t *testing.T) {
	_, f := setup(t)
	ctx := context.Background()
	for _, p := range []string{"../secret.txt", "/etc/passwd", "Films/../../secret.txt", "escape.txt", "escapedir/secret.txt"} {
		if _, err := f.Open(ctx, p); err == nil {
			t.Errorf("Open(%q) succeeded", p)
		}
		if _, err := f.Stat(ctx, p); err == nil {
			t.Errorf("Stat(%q) succeeded", p)
		}
	}
	if _, err := f.Open(ctx, "../secret.txt"); !errors.Is(err, vfs.ErrPath) {
		t.Errorf("err = %v; want ErrPath", err)
	}
	if _, err := f.Open(ctx, "escape.txt"); !errors.Is(err, vfs.ErrPath) {
		t.Errorf("symlink escape err = %v; want ErrPath", err)
	}
	// In-root symlink works.
	e, err := f.Stat(ctx, "inside.txt")
	if err != nil || e.Size != 1 {
		t.Fatalf("Stat(inside.txt) = %+v, %v", e, err)
	}
	if f.InputURL("../x") != "" {
		t.Error("InputURL accepted ../")
	}
}

func TestWalk(t *testing.T) {
	root, f := setup(t)
	ctx := context.Background()
	var got []string
	err := f.Walk(ctx, "", func(e vfs.Entry) error {
		got = append(got, e.Path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Films", "Films/Heat (1995)", "Films/Heat (1995)/heat.mkv", "Films/Heat (1995)/heat.nfo",
		"Series", "Series/S01", "Series/S01/e1.mkv", "a.txt", "inside.txt"}
	if len(got) != len(want) {
		t.Fatalf("walk = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("walk = %v; want %v", got, want)
		}
	}
	// SkipDir + sub-dir walk.
	got = nil
	err = f.Walk(ctx, "Films", func(e vfs.Entry) error {
		got = append(got, e.Path)
		if e.IsDir {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil || len(got) != 1 || got[0] != "Films/Heat (1995)" {
		t.Fatalf("skipdir walk = %v, %v", got, err)
	}
	if err := f.Walk(ctx, "nope", func(vfs.Entry) error { return nil }); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("walk missing dir: %v", err)
	}
	// Unreadable sub-directory is skipped (not meaningful as root).
	if os.Geteuid() != 0 {
		bad := filepath.Join(root, "Series", "S01")
		if err := os.Chmod(bad, 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(bad, 0o755)
		n := 0
		if err := f.Walk(ctx, "", func(vfs.Entry) error { n++; return nil }); err != nil {
			t.Fatalf("walk with unreadable dir: %v", err)
		}
		if n != len(want)-1 {
			t.Fatalf("n = %d", n)
		}
	}
}

func TestStatReadDir(t *testing.T) {
	_, f := setup(t)
	ctx := context.Background()
	e, err := f.Stat(ctx, "Films/Heat (1995)/heat.mkv")
	if err != nil || e.Size != 1000 || e.IsDir || e.Path != "Films/Heat (1995)/heat.mkv" || e.ModTime.IsZero() {
		t.Fatalf("Stat = %+v, %v", e, err)
	}
	if e, err := f.Stat(ctx, ""); err != nil || !e.IsDir {
		t.Fatalf("Stat root = %+v, %v", e, err)
	}
	if _, err := f.Stat(ctx, "missing"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
	es, err := f.ReadDir(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Path)
	}
	if len(names) != 4 || names[0] != "Films" || names[3] != "inside.txt" {
		t.Fatalf("ReadDir = %v", names)
	}
}

func TestOpenSeek(t *testing.T) {
	root, f := setup(t)
	ctx := context.Background()
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(rand.IntN(256))
	}
	write(t, filepath.Join(root, "big.bin"), data)
	fl, err := f.Open(ctx, "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer fl.Close()
	if fl.Size() != int64(len(data)) || fl.ModTime().IsZero() {
		t.Fatalf("size %d", fl.Size())
	}
	for range 50 {
		off := rand.Int64N(int64(len(data)))
		n := rand.IntN(70000)
		if _, err := fl.Seek(off, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, n)
		got, err := io.ReadFull(fl, buf)
		end := min(off+int64(n), int64(len(data)))
		if !bytes.Equal(buf[:got], data[off:end]) {
			t.Fatalf("mismatch at %d", off)
		}
		if got < n && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
	}
	if _, err := f.Open(ctx, "Films"); !errors.Is(err, vfs.ErrPath) {
		t.Fatalf("open dir: %v", err)
	}
	if got := f.InputURL("big.bin"); got != filepath.Join(root, "big.bin") {
		t.Fatalf("InputURL = %q", got)
	}
}

func TestOffline(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "media")
	f, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx := context.Background()
	if err := f.Walk(ctx, "", func(vfs.Entry) error { return nil }); !errors.Is(err, vfs.ErrOffline) || errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("walk missing root: %v (must be ErrOffline, not ErrNotExist)", err)
	}
	if err := f.Ping(ctx); !errors.Is(err, vfs.ErrOffline) {
		t.Fatalf("ping: %v", err)
	}
	// Appears later → online; empty walk is fine.
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.Walk(ctx, "", func(vfs.Entry) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Replaced directory is picked up (root reopened).
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := f.Ping(ctx); !errors.Is(err, vfs.ErrOffline) {
		t.Fatalf("ping after removal: %v", err)
	}
	write(t, filepath.Join(root, "new.mkv"), []byte("n"))
	if _, err := f.Stat(ctx, "new.mkv"); err != nil {
		t.Fatal(err)
	}
}
