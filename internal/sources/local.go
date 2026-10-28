package sources

import (
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// Plain OS access for local locations that are not attached to a known
// source (e.g. paths already stored before sources existed).

type osFile struct {
	*os.File
	size  int64
	mtime time.Time
}

func (f osFile) Size() int64        { return f.size }
func (f osFile) ModTime() time.Time { return f.mtime }

func openLocal(p string) (vfs.File, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return osFile{File: f, size: st.Size(), mtime: st.ModTime()}, nil
}

func readDirLocal(dir string) ([]vfs.Entry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]vfs.Entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, vfs.Entry{Path: e.Name(), Size: info.Size(), ModTime: info.ModTime(), IsDir: e.IsDir()})
	}
	return out, nil
}

func statLocal(p string) (vfs.Entry, error) {
	st, err := os.Stat(p)
	if err != nil {
		return vfs.Entry{}, err
	}
	return vfs.Entry{Path: filepath.Base(p), Size: st.Size(), ModTime: st.ModTime(), IsDir: st.IsDir()}, nil
}

func urlPathEscape(s string) string { return url.PathEscape(s) }
