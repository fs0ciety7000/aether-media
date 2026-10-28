package sftpfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/pkg/sftp"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

const (
	minChunk = 64 << 10
	maxChunk = 2 << 20
)

// sftpFile wraps an *sftp.File with a read-ahead buffer. Reads are served
// with ReadAt into a buffer that grows (64 KiB → 2 MiB) while access stays
// sequential, so each refill is a single pipelined burst of concurrent SFTP
// requests instead of one round trip per caller-sized Read. Seek only moves
// the logical offset; the buffer is kept, so short backward/forward seeks
// within it cost nothing. If the connection drops, the file is reopened
// once on the new connection and the read resumes at the same offset.
type sftpFile struct {
	fs   *FS
	ctx  context.Context
	full string
	size int64
	mod  time.Time

	c      *conn
	f      *sftp.File
	off    int64
	buf    []byte
	bufOff int64
	end    int64 // offset right after the previous underlying read
	chunk  int
	closed bool
}

var _ vfs.File = (*sftpFile)(nil)

// Open opens a regular file. ctx is used for reconnections during reads.
func (f *FS) Open(ctx context.Context, name string) (vfs.File, error) {
	_, full, err := f.full(name)
	if err != nil {
		return nil, err
	}
	sf := &sftpFile{fs: f, ctx: ctx, full: full, chunk: minChunk}
	if err := sf.open(true); err != nil {
		return nil, err
	}
	return sf, nil
}

func (sf *sftpFile) open(first bool) error {
	return sf.fs.do(sf.ctx, func(c *conn) error {
		if err := c.confined(sf.full); err != nil {
			return err
		}
		h, err := c.sftp.Open(sf.full)
		if err != nil {
			return err
		}
		fi, err := h.Stat()
		if err != nil {
			_ = h.Close()
			return err
		}
		if fi.IsDir() {
			_ = h.Close()
			return fmt.Errorf("%w: %s is a directory", vfs.ErrPath, sf.full)
		}
		if first {
			sf.size, sf.mod = fi.Size(), fi.ModTime()
		}
		sf.c, sf.f = c, h
		return nil
	})
}

func (sf *sftpFile) Size() int64        { return sf.size }
func (sf *sftpFile) ModTime() time.Time { return sf.mod }

// readAt reads at off, reopening the file once if the connection was lost.
func (sf *sftpFile) readAt(p []byte, off int64) (int, error) {
	for attempt := 0; ; attempt++ {
		if sf.f == nil {
			if err := sf.open(false); err != nil {
				return 0, err
			}
		}
		n, err := sf.f.ReadAt(p, off)
		if err == nil || errors.Is(err, io.EOF) || !isConnErr(err) {
			return n, err
		}
		// Connection lost: forget it and retry once on a fresh one.
		sf.fs.drop(sf.c)
		sf.f, sf.c = nil, nil
		if n > 0 {
			return n, nil
		}
		if attempt > 0 || sf.ctx.Err() != nil {
			if sf.ctx.Err() != nil {
				return 0, sf.ctx.Err()
			}
			return 0, fmt.Errorf("%w: sftp read: %w", vfs.ErrOffline, err)
		}
	}
}

func (sf *sftpFile) Read(p []byte) (int, error) {
	if sf.closed {
		return 0, errors.New("sftp: read on closed file")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if sf.off >= sf.size {
		return 0, io.EOF
	}
	if rem := sf.size - sf.off; int64(len(p)) > rem {
		p = p[:rem]
	}
	// Served from the buffer?
	if sf.off >= sf.bufOff && sf.off < sf.bufOff+int64(len(sf.buf)) {
		n := copy(p, sf.buf[sf.off-sf.bufOff:])
		sf.off += int64(n)
		return n, nil
	}
	if sf.end > 0 && sf.off == sf.end {
		sf.chunk = min(sf.chunk*2, maxChunk)
	} else {
		sf.chunk = minChunk
	}
	// Large reads go straight into the caller's buffer.
	if len(p) >= sf.chunk {
		n, err := sf.readAt(p, sf.off)
		sf.off += int64(n)
		if n > 0 {
			sf.buf, sf.end = sf.buf[:0], sf.off
			return n, nil
		}
		return 0, err
	}
	want := min(int64(sf.chunk), sf.size-sf.off)
	if int64(cap(sf.buf)) < want {
		sf.buf = make([]byte, want)
	}
	buf := sf.buf[:want]
	n, err := sf.readAt(buf, sf.off)
	sf.buf, sf.bufOff, sf.end = buf[:n], sf.off, sf.off+int64(n)
	if n == 0 {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return 0, err
	}
	c := copy(p, sf.buf)
	sf.off += int64(c)
	return c, nil
}

func (sf *sftpFile) Seek(offset int64, whence int) (int64, error) {
	if sf.closed {
		return 0, errors.New("sftp: seek on closed file")
	}
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = sf.off + offset
	case io.SeekEnd:
		abs = sf.size + offset
	default:
		return 0, errors.New("sftp: invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("sftp: negative position")
	}
	sf.off = abs
	return abs, nil
}

func (sf *sftpFile) Close() error {
	if sf.closed {
		return nil
	}
	sf.closed = true
	sf.buf = nil
	if sf.f != nil {
		err := sf.f.Close()
		sf.f = nil
		if err != nil && isConnErr(err) {
			return nil // the handle died with the connection
		}
		return err
	}
	return nil
}
