package library

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/naming"
)

// file is a media file found on disk.
type file struct {
	path  string
	rel   string // relative to the library root, '/'-separated
	root  string
	size  int64
	mtime int64
}

// Directories never worth descending into (NAS metadata, trash, OS folders).
var skipDirs = map[string]bool{
	"@eadir": true, "#recycle": true, "#snapshot": true, "$recycle.bin": true,
	"lost+found": true, "system volume information": true, ".trash": true,
	".trashes": true, ".snapshots": true, "@recycle": true,
}

// Marker files that exclude their directory (and everything below).
var ignoreMarkers = []string{".aetherignore", ".ignore", ".plexignore", ".nomedia"}

// ErrRootUnavailable means a library root could not be read. The scanner must
// not treat its files as deleted (unmounted network share, unplugged disk).
var ErrRootUnavailable = errors.New("library root unavailable")

// walk lists media files under root accepted by keep. A root that is missing
// or empty is reported as unavailable rather than as "no files".
func walk(root string, keep func(path string) bool) ([]file, error) {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) == 0 {
		return nil, ErrRootUnavailable
	}
	var files []file
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			// Unreadable sub-directory: skip it, keep scanning the rest.
			return fs.SkipDir
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)]) {
				return fs.SkipDir
			}
			for _, m := range ignoreMarkers {
				if _, err := os.Stat(filepath.Join(path, m)); err == nil {
					return fs.SkipDir
				}
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		if !keep(path) || naming.ClassifyExtra(path) == "sample" {
			return nil
		}
		info, err := os.Stat(path) // follows symlinks
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, file{
			path: path, rel: filepath.ToSlash(rel), root: root,
			size: info.Size(), mtime: info.ModTime().Unix(),
		})
		return nil
	})
	if err != nil {
		return nil, ErrRootUnavailable
	}
	return files, nil
}
