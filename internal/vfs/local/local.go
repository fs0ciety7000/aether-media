// Package local implements vfs.FS over a local directory, confined with
// os.Root so that neither ".." nor symlinks can reach outside the root.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// FS is a local directory source. The root is (re)opened lazily so that a
// directory that is missing at startup (unmounted disk) comes back by itself.
type FS struct {
	abs string

	mu   sync.Mutex
	root *os.Root
}

var _ vfs.FS = (*FS)(nil)

// New returns a source rooted at dir. A missing directory is not an error
// here: operations return vfs.ErrOffline until it appears.
func New(dir string) (vfs.FS, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: empty root", vfs.ErrPath)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("local: %w", err)
	}
	f := &FS{abs: abs}
	f.mu.Lock()
	_, _ = f.rootLocked()
	f.mu.Unlock()
	return f, nil
}

// rootLocked returns the opened root, reopening it when the directory at abs
// is no longer the one held open (unmounted, replaced, deleted).
func (f *FS) rootLocked() (*os.Root, error) {
	st, err := os.Stat(f.abs)
	if err != nil || !st.IsDir() {
		if f.root != nil {
			_ = f.root.Close()
			f.root = nil
		}
		if err == nil {
			err = errors.New("not a directory")
		}
		return nil, fmt.Errorf("%w: %s: %s", vfs.ErrOffline, f.abs, err.Error())
	}
	if f.root != nil {
		if held, err := f.root.Stat("."); err == nil && os.SameFile(st, held) {
			return f.root, nil
		}
		_ = f.root.Close()
		f.root = nil
	}
	r, err := os.OpenRoot(f.abs)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %s", vfs.ErrOffline, f.abs, err.Error())
	}
	f.root = r
	return r, nil
}

func (f *FS) getRoot() (*os.Root, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rootLocked()
}

// osName converts a cleaned relative path into an os.Root name.
func osName(rel string) string {
	if rel == "" {
		return "."
	}
	return filepath.FromSlash(rel)
}

// mapErr converts os / os.Root errors to vfs errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return err // vfs.ErrNotExist == fs.ErrNotExist
	case strings.Contains(err.Error(), "path escapes"):
		return fmt.Errorf("%w: %w", vfs.ErrPath, err)
	}
	return err
}

func entry(rel string, fi fs.FileInfo) vfs.Entry {
	e := vfs.Entry{Path: rel, ModTime: fi.ModTime(), IsDir: fi.IsDir()}
	if !e.IsDir {
		e.Size = fi.Size()
	}
	return e
}

// Walk lists every entry below dir (dir itself excluded), parents first, in
// lexical order. Unreadable sub-directories and dangling or escaping
// symlinks are skipped.
func (f *FS) Walk(ctx context.Context, dir string, fn func(vfs.Entry) error) error {
	rel, err := vfs.CleanRel(dir)
	if err != nil {
		return err
	}
	r, err := f.getRoot()
	if err != nil {
		return err
	}
	start := osName(rel)
	if fi, err := r.Stat(start); err != nil {
		return mapErr(err)
	} else if !fi.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", vfs.ErrPath, dir)
	}
	err = fs.WalkDir(r.FS(), path.Clean("./"+rel), func(p string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if p == path.Clean("./"+rel) {
				return err
			}
			// Unreadable directory (second call) or vanished entry: skip.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == path.Clean("./"+rel) {
			return nil
		}
		var fi fs.FileInfo
		if d.Type()&fs.ModeSymlink != 0 {
			// Resolve through the root: escaping or dangling links are refused.
			fi, err = r.Stat(filepath.FromSlash(p))
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				// WalkDir does not descend into symlinked dirs; report the
				// directory but do not follow (avoids cycles).
				return fn(entry(p, fi))
			}
		} else {
			fi, err = d.Info()
			if err != nil {
				return nil
			}
			if !fi.IsDir() && !fi.Mode().IsRegular() {
				return nil // sockets, devices, fifos
			}
		}
		return fn(entry(p, fi))
	})
	if errors.Is(err, fs.SkipAll) {
		return nil
	}
	return mapErr(err)
}

// Stat describes name.
func (f *FS) Stat(_ context.Context, name string) (vfs.Entry, error) {
	rel, err := vfs.CleanRel(name)
	if err != nil {
		return vfs.Entry{}, err
	}
	r, err := f.getRoot()
	if err != nil {
		return vfs.Entry{}, err
	}
	fi, err := r.Stat(osName(rel))
	if err != nil {
		return vfs.Entry{}, mapErr(err)
	}
	return entry(rel, fi), nil
}

// ReadDir lists the direct children of dir, sorted by name.
func (f *FS) ReadDir(_ context.Context, dir string) ([]vfs.Entry, error) {
	rel, err := vfs.CleanRel(dir)
	if err != nil {
		return nil, err
	}
	r, err := f.getRoot()
	if err != nil {
		return nil, err
	}
	des, err := fs.ReadDir(r.FS(), path.Clean("./"+rel))
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]vfs.Entry, 0, len(des))
	for _, d := range des {
		p := vfs.Join(rel, d.Name())
		fi, err := r.Stat(osName(p)) // follows in-root symlinks only
		if err != nil {
			continue
		}
		if !fi.IsDir() && !fi.Mode().IsRegular() {
			continue
		}
		out = append(out, entry(p, fi))
	}
	return out, nil
}

type file struct {
	*os.File
	size int64
	mod  fs.FileInfo
}

func (f *file) Size() int64 { return f.size }

func (f *file) ModTime() time.Time { return f.mod.ModTime() }

// Open opens a regular file for reading.
func (f *FS) Open(_ context.Context, name string) (vfs.File, error) {
	rel, err := vfs.CleanRel(name)
	if err != nil {
		return nil, err
	}
	r, err := f.getRoot()
	if err != nil {
		return nil, err
	}
	of, err := r.Open(osName(rel))
	if err != nil {
		return nil, mapErr(err)
	}
	fi, err := of.Stat()
	if err != nil {
		_ = of.Close()
		return nil, err
	}
	if fi.IsDir() {
		_ = of.Close()
		return nil, fmt.Errorf("%w: %s is a directory", vfs.ErrPath, name)
	}
	return &file{File: of, size: fi.Size(), mod: fi}, nil
}

// InputURL returns the absolute OS path of name ("" if the path is invalid).
func (f *FS) InputURL(name string) string {
	rel, err := vfs.CleanRel(name)
	if err != nil {
		return ""
	}
	return filepath.Join(f.abs, filepath.FromSlash(rel))
}

// Ping checks the root directory exists and is readable.
func (f *FS) Ping(_ context.Context) error {
	r, err := f.getRoot()
	if err != nil {
		return err
	}
	d, err := r.Open(".")
	if err != nil {
		return fmt.Errorf("%w: %s", vfs.ErrOffline, err.Error())
	}
	defer func() { _ = d.Close() }()
	if _, err := d.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: %s", vfs.ErrOffline, err.Error())
	}
	return nil
}

// Close releases the root handle.
func (f *FS) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.root == nil {
		return nil
	}
	err := f.root.Close()
	f.root = nil
	return err
}
