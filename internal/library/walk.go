package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/naming"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// file is a media file found in a source.
type file struct {
	path     string // stored location (absolute path or "src:<id>:/<rel>")
	rel      string // relative to the source root, '/'-separated
	root     string // location of the source root
	sourceID int64
	size     int64
	mtime    int64
}

// Directories never worth descending into (NAS metadata, trash, OS folders).
var skipDirs = map[string]bool{
	"@eadir": true, "#recycle": true, "#snapshot": true, "$recycle.bin": true,
	"lost+found": true, "system volume information": true, ".trash": true,
	".trashes": true, ".snapshots": true, "@recycle": true,
}

// Marker files that exclude their directory (and everything below).
var ignoreMarkers = map[string]bool{".aetherignore": true, ".ignore": true, ".plexignore": true, ".nomedia": true}

// ErrRootUnavailable means a source could not be listed. The scanner must
// not treat its files as deleted (offline agent, unmounted share...).
var ErrRootUnavailable = errors.New("library root unavailable")

// walkStats counts files left out of a walk.
type walkStats struct {
	discImages []string // disc images (.iso), not supported: relative paths
}

// walk lists the media files of a source accepted by keep. A source that
// cannot be listed, or lists nothing at all, is reported as unavailable
// rather than as "no files". Disc images are never kept (v0.1 cannot play
// them): they are counted in stats instead of being probed, failing, and
// shown as unreadable files.
func walk(ctx context.Context, fsys vfs.FS, src db.LibrarySource, keep func(name string) bool, stats *walkStats) ([]file, error) {
	var found []file
	var ignored []string // directories holding an ignore marker
	sawAny := false
	err := fsys.Walk(ctx, "", func(e vfs.Entry) error {
		if e.Path == "" {
			return nil
		}
		sawAny = true
		name := path.Base(e.Path)
		if e.IsDir {
			if strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)] {
				return fs.SkipDir
			}
			return nil
		}
		if ignoreMarkers[strings.ToLower(name)] {
			ignored = append(ignored, path.Dir(e.Path))
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if naming.IsDiscImage(name) {
			if stats != nil {
				stats.discImages = append(stats.discImages, e.Path)
			}
			return nil
		}
		if !keep(name) || naming.ClassifyExtra(e.Path) == "sample" {
			return nil
		}
		found = append(found, file{
			path: sources.Location(src, e.Path), rel: e.Path, root: sources.RootLocation(src),
			sourceID: src.ID, size: e.Size, mtime: e.ModTime.Unix(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRootUnavailable, err)
	}
	if !sawAny {
		return nil, fmt.Errorf("%w: dossier vide", ErrRootUnavailable)
	}
	if len(ignored) == 0 {
		return found, nil
	}
	out := found[:0]
	for _, f := range found {
		if !underIgnored(f.rel, ignored) {
			out = append(out, f)
		}
	}
	return out, nil
}

func underIgnored(rel string, dirs []string) bool {
	for _, d := range dirs {
		if d == "." || rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}
