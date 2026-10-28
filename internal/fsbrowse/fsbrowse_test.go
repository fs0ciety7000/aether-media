package fsbrowse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestList(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"Films", "séries", ".cache", "@eaDir", "lost+found"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o644)
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "escape"))

	dirs, err := List([]string{root}, root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range dirs {
		names = append(names, d.Name)
	}
	if len(names) != 3 || names[0] != "escape" || names[1] != "Films" || names[2] != "séries" {
		t.Fatalf("names = %v", names)
	}
	// Following the symlink leaves the roots: denied.
	if _, err := List([]string{root}, filepath.Join(root, "escape")); err != ErrDenied {
		t.Errorf("escape → %v", err)
	}
	if _, err := List([]string{root}, "relative"); err != ErrDenied {
		t.Errorf("relative → %v", err)
	}
	if _, err := List([]string{root}, filepath.Join(root, "..")); err != ErrDenied {
		t.Errorf("parent → %v", err)
	}
	if r, _ := List([]string{root}, ""); len(r) != 1 || r[0].Path != root {
		t.Errorf("roots = %v", r)
	}
}
