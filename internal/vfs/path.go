package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// CleanRel normalises a slash-separated path relative to a source root.
// "" and "." denote the root and yield "". Leading slashes, backslashes,
// NUL bytes and any ".." element are refused with ErrPath.
func CleanRel(p string) (string, error) {
	if p == "" || p == "." {
		return "", nil
	}
	if strings.ContainsRune(p, 0) || strings.ContainsRune(p, '\\') {
		return "", fmt.Errorf("%w: %q", ErrPath, p)
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%w: absolute path %q", ErrPath, p)
	}
	for _, el := range strings.Split(p, "/") {
		if el == ".." {
			return "", fmt.Errorf("%w: %q escapes root", ErrPath, p)
		}
	}
	c := path.Clean(p)
	if c == "." {
		return "", nil
	}
	return c, nil
}

// Join joins a cleaned relative path onto root (both slash-separated).
// rel must already have been validated with CleanRel.
func Join(root, rel string) string {
	switch {
	case rel == "":
		return root
	case root == "":
		return rel
	}
	return strings.TrimSuffix(root, "/") + "/" + rel
}

// ErrTooLarge is returned by ReadFileLimit when the file exceeds the limit.
var ErrTooLarge = errors.New("vfs: file too large")

// ReadFileLimit reads a whole (small) file such as an NFO, refusing files
// larger than limit bytes.
func ReadFileLimit(ctx context.Context, fsys FS, name string, limit int64) ([]byte, error) {
	f, err := fsys.Open(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if sz := f.Size(); sz > limit {
		return nil, fmt.Errorf("%w: %s (%d > %d bytes)", ErrTooLarge, name, sz, limit)
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, name)
	}
	return b, nil
}
