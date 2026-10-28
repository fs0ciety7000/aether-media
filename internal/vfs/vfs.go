// Package vfs abstracts where media files live: a local directory, a remote
// Aether agent or an SFTP server. Paths are slash-separated and relative to
// the source root ("" is the root itself); implementations must refuse any
// path escaping the root.
package vfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"
)

// Entry describes a file or directory.
type Entry struct {
	Path    string    `json:"path"` // relative to the FS root, slash-separated
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
	IsDir   bool      `json:"dir,omitempty"`
}

// File is an open file supporting random access (HTTP Range, ffprobe).
type File interface {
	io.ReadSeekCloser
	Size() int64
	ModTime() time.Time
}

// FS is a media source.
type FS interface {
	// Walk lists entries under dir (recursive, parents before children).
	// Returning fs.SkipDir from fn for a directory skips its contents.
	Walk(ctx context.Context, dir string, fn func(Entry) error) error
	Stat(ctx context.Context, name string) (Entry, error)
	ReadDir(ctx context.Context, dir string) ([]Entry, error)
	Open(ctx context.Context, name string) (File, error)
	// InputURL returns something FFmpeg/ffprobe can read directly: a local
	// path or an http(s) URL with credentials embedded. "" means the caller
	// must expose the file itself (see the server's internal proxy).
	InputURL(name string) string
	// Ping checks the source is reachable.
	Ping(ctx context.Context) error
	Close() error
}

var (
	// ErrNotExist is returned for missing files (wraps fs.ErrNotExist).
	ErrNotExist = fs.ErrNotExist
	// ErrOffline means the source cannot be reached right now.
	ErrOffline = errors.New("vfs: source offline")
	// ErrPath means a path escapes the source root or is malformed.
	ErrPath = errors.New("vfs: invalid path")
)
