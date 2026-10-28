package images

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"strings"
	"testing"
)

// TestIngestReader: artwork of remote sources is read through their file
// system, never fetched over HTTP (the fetcher refuses non-public hosts).
func TestIngestReader(t *testing.T) {
	c := newCache(t)
	data := encPNG(t, solid(40, 60, color.NRGBA{R: 200, G: 30, B: 30, A: 255}))
	s, err := c.IngestReader(context.Background(), bytes.NewReader(data))
	if err != nil || s.Width != 40 || s.Height != 60 || s.Hash == "" {
		t.Fatalf("ingest: %+v %v", s, err)
	}
	small := newCache(t, func(o *Options) { o.MaxBytes = 10 })
	if _, err := small.IngestReader(context.Background(), bytes.NewReader(data)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	if _, err := c.IngestReader(context.Background(), strings.NewReader("pas une image")); err == nil {
		t.Fatal("garbage accepted")
	}
}
