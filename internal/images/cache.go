// Package images implements Aether's image cache: it fetches artwork (remote URLs
// or local files), stores the original bytes content-addressed on disk, analyses
// them (dimensions, blurhash, dominant colour) and serves resized renditions.
package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/netguard"
)

// Sentinel errors. Use errors.Is.
var (
	ErrNotFound    = errors.New("images: not found")
	ErrNotImage    = errors.New("images: not a supported image")
	ErrTooLarge    = errors.New("images: image too large")
	ErrBadSource   = errors.New("images: invalid source")
	ErrInvalidHash = errors.New("images: invalid hash")
)

const defaultMaxBytes = 25 << 20

// Options configures a Cache.
type Options struct {
	Dir string // cache root, e.g. <data>/cache/images
	// HTTPClient downloads remote images. The default (netguard) only reaches
	// public addresses, checked at connect time and on every redirect, with
	// timeouts; a custom client bypasses that protection.
	HTTPClient *http.Client
	// AllowNetworks are exceptions to that protection (e.g. a LAN artwork
	// server, or tests on loopback). Ignored with a custom HTTPClient.
	AllowNetworks []netip.Prefix
	UserAgent     string
	MaxBytes      int64 // max source size, default 25 MiB
}

// Source describes an ingested original image.
type Source struct {
	Hash          string // sha256 hex of the original bytes
	Path          string // absolute path of the stored original
	Format        string // "jpeg" | "png" | "webp" | "gif"
	Width, Height int    // display dimensions (EXIF orientation applied)
	Blurhash      string // 4x3 (landscape) or 3x4 (portrait) components
	Dominant      string // "#rrggbb"
	HasAlpha      bool
}

// Cache is safe for concurrent use.
type Cache struct {
	dir      string
	client   *http.Client
	ua       string
	maxBytes int64

	mu       sync.Mutex
	inflight map[string]*flight
	sem      chan struct{} // bounds concurrent decodes/renders (memory)
	renders  atomic.Int64  // number of variants actually rendered (for tests)
}

// New creates the cache directories and returns a Cache.
func New(o Options) (*Cache, error) {
	if o.Dir == "" {
		return nil, errors.New("images: Options.Dir is required")
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return nil, err
	}
	for _, d := range []string{dir, filepath.Join(dir, "originals"), filepath.Join(dir, "variants")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	c := &Cache{
		dir:      dir,
		client:   o.HTTPClient,
		ua:       o.UserAgent,
		maxBytes: o.MaxBytes,
		inflight: make(map[string]*flight),
		sem:      make(chan struct{}, max(2, runtime.GOMAXPROCS(0))),
	}
	if c.client == nil {
		c.client = netguard.NewClient(netguard.ClientOptions{Timeout: 30 * time.Second, Allow: o.AllowNetworks})
	}
	if c.maxBytes <= 0 {
		c.maxBytes = defaultMaxBytes
	}
	return c, nil
}

// Ingest fetches src (http(s) URL or absolute local path), validates it is a real
// image, stores the original content-addressed and returns its analysis.
// Ingesting identical bytes twice is a no-op on disk and returns the same Source.
func (c *Cache) Ingest(ctx context.Context, src string) (Source, error) {
	data, err := c.fetch(ctx, src)
	if err != nil {
		return Source{}, err
	}
	return c.ingest(ctx, data)
}

// IngestReader is Ingest for an image read from r (artwork of a remote
// source, read through its file system rather than over HTTP).
func (c *Cache) IngestReader(ctx context.Context, r io.Reader) (Source, error) {
	data, err := io.ReadAll(io.LimitReader(r, c.maxBytes+1))
	if err != nil {
		return Source{}, err
	}
	if int64(len(data)) > c.maxBytes {
		return Source{}, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, c.maxBytes)
	}
	return c.ingest(ctx, data)
}

func (c *Cache) ingest(ctx context.Context, data []byte) (Source, error) {
	hdr, err := readHeader(data)
	if err != nil {
		return Source{}, err
	}
	if err := c.acquire(ctx); err != nil {
		return Source{}, err
	}
	defer c.release()
	img, err := decodeFull(data)
	if err != nil {
		return Source{}, err
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	path := c.originalPath(hash, allowedFormats[hdr.format])
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Source{}, err
		}
		if err := writeAtomic(path, func(w io.Writer) error { _, err := w.Write(data); return err }); err != nil {
			return Source{}, err
		}
	}

	thumb := thumbnail(img, hdr)
	nx, ny := blurhashComponents(thumb.Rect.Dx(), thumb.Rect.Dy())
	w, h := hdr.oriented()
	return Source{
		Hash:     hash,
		Path:     path,
		Format:   hdr.format,
		Width:    w,
		Height:   h,
		Blurhash: encodeBlurhash(thumb, nx, ny),
		Dominant: dominantColour(thumb),
		HasAlpha: hasAlpha(img),
	}, nil
}

func (c *Cache) fetch(ctx context.Context, src string) ([]byte, error) {
	if filepath.IsAbs(src) {
		return c.readLocal(src)
	}
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: %q (want http(s) URL or absolute path)", ErrBadSource, src)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadSource, err)
	}
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/*;q=0.8")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, netguard.ErrBlocked) {
			return nil, fmt.Errorf("%w: %s: %w", ErrBadSource, u.Redacted(), err)
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return nil, fmt.Errorf("%w: %s: HTTP %d", ErrNotFound, u.Redacted(), resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return nil, fmt.Errorf("images: fetch %s: HTTP %d", u.Redacted(), resp.StatusCode)
	}
	if resp.ContentLength > c.maxBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrTooLarge, resp.ContentLength)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > c.maxBytes {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, c.maxBytes)
	}
	return data, nil
}

func (c *Cache) readLocal(p string) ([]byte, error) {
	p = filepath.Clean(p)
	fi, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, p)
		}
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrBadSource, p)
	}
	if fi.Size() > c.maxBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrTooLarge, fi.Size())
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, c.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > c.maxBytes {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, c.maxBytes)
	}
	return data, nil
}

// Remove deletes the original and all its variants. Removing an unknown hash is not an error.
func (c *Cache) Remove(hash string) error {
	if !validHash(hash) {
		return ErrInvalidHash
	}
	var errs []error
	for _, ext := range allowedFormats {
		if err := os.Remove(c.originalPath(hash, ext)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(c.dir, "variants", hash[:2], hash+"-*"))
	if err != nil {
		errs = append(errs, err)
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Cache) originalPath(hash, ext string) string {
	return filepath.Join(c.dir, "originals", hash[:2], hash[2:4], hash+"."+ext)
}

// findOriginal locates the stored original for hash.
func (c *Cache) findOriginal(hash string) (string, error) {
	for _, ext := range []string{"jpg", "png", "webp", "gif"} {
		p := c.originalPath(hash, ext)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrNotFound, hash)
}

func (c *Cache) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Cache) release() { <-c.sem }

func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for i := 0; i < len(h); i++ {
		ch := h[i]
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

// writeAtomic writes via a temp file in the destination directory and renames it into place.
func writeAtomic(path string, fill func(io.Writer) error) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = fill(tmp); err != nil {
		return err
	}
	if err = tmp.Chmod(0o644); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
