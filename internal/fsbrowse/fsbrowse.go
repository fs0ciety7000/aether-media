// Package fsbrowse lists directories for the folder pickers of the web UI
// (server-side local sources, and agents choosing what to share). Browsing
// is confined to a set of roots where media usually lives.
package fsbrowse

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Dir is a directory offered by the picker.
type Dir struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ErrDenied is returned for paths outside the browse roots.
var ErrDenied = errors.New("fsbrowse: path outside the allowed folders")

// DefaultRoots are the usual places for media on each OS (only those that
// exist are kept).
func DefaultRoots() []string {
	var cands []string
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		for c := 'C'; c <= 'Z'; c++ {
			cands = append(cands, string(c)+`:\`)
		}
	case "darwin":
		cands = []string{home, "/Volumes"}
	default:
		cands = []string{home, "/mnt", "/media", "/srv", "/data", "/storage", "/volume1", "/volume2", "/share", "/run/media"}
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range cands {
		if c == "" || seen[c] {
			continue
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// Allowed resolves p (absolute, symlinks evaluated) and checks it lies within
// one of roots.
func Allowed(roots []string, p string) (string, bool) {
	if !filepath.IsAbs(p) {
		return "", false
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(p))
	if err != nil {
		return "", false
	}
	for _, r := range roots {
		rr, err := filepath.EvalSymlinks(r)
		if err != nil {
			continue
		}
		if real == rr || strings.HasPrefix(real, strings.TrimSuffix(rr, string(filepath.Separator))+string(filepath.Separator)) {
			return real, true
		}
	}
	return "", false
}

// List returns the roots when p is empty, else the visible sub-directories
// of p (sorted, hidden and system folders skipped).
func List(roots []string, p string) ([]Dir, error) {
	if p == "" {
		out := make([]Dir, 0, len(roots))
		for _, r := range roots {
			out = append(out, Dir{Name: r, Path: r})
		}
		return out, nil
	}
	real, ok := Allowed(roots, p)
	if !ok {
		return nil, ErrDenied
	}
	entries, err := os.ReadDir(real)
	if err != nil {
		return nil, err
	}
	out := []Dir{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "$") ||
			strings.EqualFold(name, "System Volume Information") || name == "lost+found" {
			continue
		}
		full := filepath.Join(real, name)
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(full); err == nil {
				isDir = st.IsDir()
			}
		}
		if isDir {
			out = append(out, Dir{Name: name, Path: full})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
