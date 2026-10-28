package nfo

import (
	"io"
	"os"
	"path/filepath"
)

// DirEntry is a directory entry as seen by the provider.
type DirEntry struct {
	Name  string
	IsDir bool
}

// FileSystem is how the provider reads NFO files and artwork. The default
// reads the local disk; remote sources (agent, SFTP) plug in their own via
// SetFileSystem, resolving the same location strings the scanner stores.
type FileSystem interface {
	ReadDir(dir string) ([]DirEntry, error)
	IsDir(path string) (bool, error)
	Open(path string) (io.ReadCloser, error)
}

var files FileSystem = osFS{}

// SetFileSystem replaces the file system used by the provider. Call it once
// at start-up, before any lookup.
func SetFileSystem(f FileSystem) { files = f }

// OSFileSystem returns the local-disk implementation (for composition).
func OSFileSystem() FileSystem { return osFS{} }

type osFS struct{}

func (osFS) ReadDir(dir string) ([]DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(filepath.Join(dir, e.Name())); err == nil {
				isDir = fi.IsDir()
			}
		}
		out = append(out, DirEntry{Name: e.Name(), IsDir: isDir})
	}
	return out, nil
}

func (osFS) IsDir(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return fi.IsDir(), nil
}

func (osFS) Open(path string) (io.ReadCloser, error) { return os.Open(path) }
