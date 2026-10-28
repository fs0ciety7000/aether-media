package images

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
)

// AllowedWidths are the widths variants are snapped to (bounds disk usage).
var AllowedWidths = []int{92, 154, 185, 342, 500, 780, 1280, 1920, 3840}

const jpegQuality = 82

// snapWidth maps a requested width to the next allowed size up (the largest allowed
// size if w exceeds them all), never larger than the source width. w <= 0 means
// "full size".
func snapWidth(w, srcW int) int {
	if w <= 0 {
		return srcW
	}
	t := AllowedWidths[len(AllowedWidths)-1]
	for _, s := range AllowedWidths {
		if s >= w {
			t = s
			break
		}
	}
	return min(t, srcW)
}

type flight struct {
	done     chan struct{}
	path, ct string
	err      error
}

// Variant returns the path and content type of a rendition of hash at (snapped)
// width w, rendering it on first use. Concurrent calls for the same rendition share
// one render.
func (c *Cache) Variant(ctx context.Context, hash string, w int) (string, string, error) {
	if !validHash(hash) {
		return "", "", ErrInvalidHash
	}
	orig, err := c.findOriginal(hash)
	if err != nil {
		return "", "", err
	}
	hdr, err := headerFromFile(orig)
	if err != nil {
		return "", "", err
	}
	srcW, _ := hdr.oriented()
	ew := snapWidth(w, srcW)
	if p, ct, ok := c.existingVariant(hash, ew); ok {
		return p, ct, nil
	}

	key := hash + "-" + strconv.Itoa(ew)
	c.mu.Lock()
	if f, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.path, f.ct, f.err
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	c.inflight[key] = f
	c.mu.Unlock()

	// Detach from the caller's cancellation: waiters share this render and the
	// result is cached for everyone anyway.
	f.path, f.ct, f.err = c.render(context.WithoutCancel(ctx), orig, hash, ew)

	c.mu.Lock()
	delete(c.inflight, key)
	c.mu.Unlock()
	close(f.done)
	return f.path, f.ct, f.err
}

func (c *Cache) variantPath(hash string, w int, ext string) string {
	return filepath.Join(c.dir, "variants", hash[:2], hash+"-"+strconv.Itoa(w)+"."+ext)
}

func (c *Cache) existingVariant(hash string, w int) (string, string, bool) {
	for _, v := range []struct{ ext, ct string }{{"jpg", "image/jpeg"}, {"png", "image/png"}} {
		p := c.variantPath(hash, w, v.ext)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, v.ct, true
		}
	}
	return "", "", false
}

func (c *Cache) render(ctx context.Context, orig, hash string, w int) (string, string, error) {
	// A previous flight may have finished between our existence check and now.
	if p, ct, ok := c.existingVariant(hash, w); ok {
		return p, ct, nil
	}
	if err := c.acquire(ctx); err != nil {
		return "", "", err
	}
	defer c.release()

	data, err := os.ReadFile(orig)
	if err != nil {
		return "", "", err
	}
	hdr, err := readHeader(data)
	if err != nil {
		return "", "", err
	}
	img, err := decodeFull(data)
	if err != nil {
		return "", "", err
	}
	ow, oh := hdr.oriented()
	h := max(1, int(math.Round(float64(oh)*float64(w)/float64(ow))))
	var out *image.RGBA
	if w == ow && hdr.orientation == 1 {
		out = image.NewRGBA(image.Rect(0, 0, ow, oh))
		draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	} else {
		out = scaleOriented(img, hdr.orientation, w, h, draw.CatmullRom)
	}

	alpha := hasAlpha(img)
	ext, ct := "jpg", "image/jpeg"
	if alpha {
		ext, ct = "png", "image/png"
	}
	path := c.variantPath(hash, w, ext)
	err = writeAtomic(path, func(wr io.Writer) error {
		if alpha {
			return (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(wr, out)
		}
		return jpeg.Encode(wr, out, &jpeg.Options{Quality: jpegQuality})
	})
	if err != nil {
		return "", "", err
	}
	c.renders.Add(1)
	return path, ct, nil
}

// headerFromFile reads only the beginning of the file (enough for DecodeConfig and EXIF).
func headerFromFile(p string) (header, error) {
	f, err := os.Open(p)
	if err != nil {
		return header{}, err
	}
	defer func() { _ = f.Close() }()
	prefix := make([]byte, 256<<10)
	n, err := io.ReadFull(f, prefix)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return header{}, err
	}
	prefix = prefix[:n]
	cfg, format, err := image.DecodeConfig(io.MultiReader(bytes.NewReader(prefix), f))
	if err != nil {
		return header{}, fmt.Errorf("%w: %w", ErrNotImage, err)
	}
	h := header{format: format, rawW: cfg.Width, rawH: cfg.Height, orientation: 1}
	if format == "jpeg" {
		h.orientation = jpegOrientation(prefix)
	}
	if h.rawW <= 0 || h.rawH <= 0 {
		return header{}, fmt.Errorf("%w: empty image", ErrNotImage)
	}
	return h, nil
}

// ServeVariant writes the rendition of hash at width to w with long-lived immutable
// caching headers and ETag "<hash>-<width>" (If-None-Match → 304).
func (c *Cache) ServeVariant(w http.ResponseWriter, r *http.Request, hash string, width int) {
	if !validHash(hash) {
		http.Error(w, "invalid image hash", http.StatusBadRequest)
		return
	}
	width = max(width, 0)
	etag := `"` + hash + "-" + strconv.Itoa(width) + `"`
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	if etagMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	path, ct, err := c.Variant(r.Context(), hash, width)
	if err != nil {
		h.Del("ETag")
		h.Del("Cache-Control")
		switch {
		case errors.Is(err, ErrNotFound):
			http.Error(w, "image not found", http.StatusNotFound)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			http.Error(w, "request cancelled", http.StatusServiceUnavailable)
		default:
			http.Error(w, "image unavailable", http.StatusInternalServerError)
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		h.Del("ETag")
		h.Del("Cache-Control")
		http.Error(w, "image unavailable", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		h.Del("ETag")
		h.Del("Cache-Control")
		http.Error(w, "image unavailable", http.StatusInternalServerError)
		return
	}
	h.Set("Content-Type", ct)
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

// etagMatch implements the weak comparison used by If-None-Match.
func etagMatch(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}
