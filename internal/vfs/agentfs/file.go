package agentfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// httpFile is a vfs.File backed by Range requests. Reads stream from a
// single open-ended "Range: bytes=<off>-" response; Seek only records the
// new offset (small forward seeks are served by discarding from the open
// body), so http.ServeContent and ffprobe access patterns cost one request
// per discontinuity.
type httpFile struct {
	fs   *FS
	ctx  context.Context
	url  string
	size int64
	mod  time.Time

	off     int64         // logical offset of the next Read
	body    io.ReadCloser // open response body, positioned at bodyOff
	bodyOff int64
	closed  bool
}

var _ vfs.File = (*httpFile)(nil)

// Open opens name for reading. ctx bounds the life of the returned file:
// every subsequent request it issues uses it.
func (f *FS) Open(ctx context.Context, name string) (vfs.File, error) {
	_, remote, err := f.remote(name)
	if err != nil {
		return nil, err
	}
	hf := &httpFile{fs: f, ctx: ctx, url: f.endpoint("/file", f.query(remote))}
	hctx, cancel := withMetaTimeout(ctx)
	defer cancel()
	resp, err := f.do(hctx, http.MethodHead, hf.url, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusMethodNotAllowed {
			_ = resp.Body.Close()
			return hf.openWithGet()
		}
		return nil, statusErr(resp)
	}
	_ = resp.Body.Close()
	hf.size = resp.ContentLength
	if hf.size < 0 {
		return nil, fmt.Errorf("%w: agent: missing Content-Length", vfs.ErrOffline)
	}
	hf.mod, _ = http.ParseTime(resp.Header.Get("Last-Modified"))
	return hf, nil
}

// openWithGet is the fallback when HEAD is not allowed: the first GET gives
// the metadata and its body is kept for the first Read.
func (hf *httpFile) openWithGet() (vfs.File, error) {
	resp, err := hf.get(0)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		_, _, total, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("%w: agent: bad Content-Range", vfs.ErrOffline)
		}
		hf.size = total
	case http.StatusOK:
		hf.size = resp.ContentLength
	case http.StatusRequestedRangeNotSatisfiable:
		_ = resp.Body.Close()
		hf.mod, _ = http.ParseTime(resp.Header.Get("Last-Modified"))
		return hf, nil // empty file
	default:
		return nil, statusErr(resp)
	}
	hf.mod, _ = http.ParseTime(resp.Header.Get("Last-Modified"))
	hf.body, hf.bodyOff = resp.Body, 0
	return hf, nil
}

func (hf *httpFile) get(off int64) (*http.Response, error) {
	h := http.Header{"Range": {"bytes=" + strconv.FormatInt(off, 10) + "-"}}
	return hf.fs.do(hf.ctx, http.MethodGet, hf.url, h)
}

func (hf *httpFile) Size() int64        { return hf.size }
func (hf *httpFile) ModTime() time.Time { return hf.mod }

func (hf *httpFile) dropBody() {
	if hf.body != nil {
		_ = hf.body.Close()
		hf.body = nil
	}
}

// reopen starts a new ranged GET at hf.off. It returns io.EOF when the
// offset is at or past the end.
func (hf *httpFile) reopen() error {
	hf.dropBody()
	if hf.off >= hf.size {
		return io.EOF
	}
	resp, err := hf.get(hf.off)
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		start, _, _, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || start != hf.off {
			_ = resp.Body.Close()
			return fmt.Errorf("%w: agent: unexpected Content-Range %q", vfs.ErrOffline, resp.Header.Get("Content-Range"))
		}
		hf.body, hf.bodyOff = resp.Body, start
	case http.StatusOK:
		// Range ignored: skip to the offset.
		if hf.off > 0 {
			if _, err := io.CopyN(io.Discard, resp.Body, hf.off); err != nil {
				_ = resp.Body.Close()
				return fmt.Errorf("%w: agent: %w", vfs.ErrOffline, err)
			}
		}
		hf.body, hf.bodyOff = resp.Body, hf.off
	case http.StatusRequestedRangeNotSatisfiable:
		_ = resp.Body.Close()
		return io.EOF
	default:
		return statusErr(resp)
	}
	return nil
}

func (hf *httpFile) Read(p []byte) (int, error) {
	if hf.closed {
		return 0, errors.New("agentfs: read on closed file")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if hf.off >= hf.size {
		hf.dropBody()
		return 0, io.EOF
	}
	if hf.body != nil && hf.bodyOff != hf.off {
		// Pending small forward seek: discard from the open body.
		if d := hf.off - hf.bodyOff; d > 0 && d <= maxSkipForward {
			if n, err := io.CopyN(io.Discard, hf.body, d); err != nil {
				hf.bodyOff += n
				hf.dropBody()
			} else {
				hf.bodyOff += n
			}
		} else {
			hf.dropBody()
		}
	}
	retried := false
	for {
		if hf.body == nil {
			if err := hf.reopen(); err != nil {
				return 0, err
			}
		}
		if rem := hf.size - hf.off; int64(len(p)) > rem {
			p = p[:rem]
		}
		n, err := hf.body.Read(p)
		hf.off += int64(n)
		hf.bodyOff += int64(n)
		if err == nil {
			return n, nil
		}
		hf.dropBody()
		if errors.Is(err, io.EOF) && hf.off >= hf.size {
			if n > 0 {
				return n, nil
			}
			return 0, io.EOF
		}
		if n > 0 {
			// Return what we have; the next Read reopens at the new offset.
			return n, nil
		}
		if hf.ctx.Err() != nil {
			return 0, hf.ctx.Err()
		}
		if retried {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return 0, fmt.Errorf("%w: agent: read: %w", vfs.ErrOffline, err)
		}
		retried = true // stream broke (or ended early): reopen once
	}
}

func (hf *httpFile) Seek(offset int64, whence int) (int64, error) {
	if hf.closed {
		return 0, errors.New("agentfs: seek on closed file")
	}
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = hf.off + offset
	case io.SeekEnd:
		abs = hf.size + offset
	default:
		return 0, errors.New("agentfs: invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("agentfs: negative position")
	}
	hf.off = abs
	if hf.body != nil {
		if d := abs - hf.bodyOff; d < 0 || d > maxSkipForward {
			hf.dropBody()
		}
	}
	return abs, nil
}

func (hf *httpFile) Close() error {
	hf.dropBody()
	hf.closed = true
	return nil
}

// parseContentRange parses "bytes start-end/total".
func parseContentRange(s string) (start, end, total int64, ok bool) {
	s, found := strings.CutPrefix(s, "bytes ")
	if !found {
		return 0, 0, 0, false
	}
	rng, tot, found := strings.Cut(s, "/")
	if !found {
		return 0, 0, 0, false
	}
	a, b, found := strings.Cut(rng, "-")
	if !found {
		return 0, 0, 0, false
	}
	var err error
	if start, err = strconv.ParseInt(a, 10, 64); err != nil {
		return 0, 0, 0, false
	}
	if end, err = strconv.ParseInt(b, 10, 64); err != nil {
		return 0, 0, 0, false
	}
	total = -1
	if tot != "*" {
		if total, err = strconv.ParseInt(tot, 10, 64); err != nil {
			return 0, 0, 0, false
		}
	}
	return start, end, total, true
}
