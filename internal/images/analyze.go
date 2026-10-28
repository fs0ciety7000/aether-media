package images

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"

	// Registered decoders: the only formats Ingest accepts.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the WebP decoder
)

const (
	maxSide     = 12000
	maxPixels   = 80_000_000
	thumbMaxDim = 64
)

var allowedFormats = map[string]string{ // format -> file extension
	"jpeg": "jpg",
	"png":  "png",
	"webp": "webp",
	"gif":  "gif",
}

// header is what we learn about an encoded image without decoding its pixels.
type header struct {
	format      string
	rawW, rawH  int // as stored
	orientation int // EXIF orientation 1..8 (JPEG only; 1 otherwise)
}

// oriented returns the display dimensions (after applying EXIF orientation).
func (h header) oriented() (int, int) {
	if h.orientation >= 5 {
		return h.rawH, h.rawW
	}
	return h.rawW, h.rawH
}

// readHeader validates the format and dimensions using image.DecodeConfig only,
// so oversized images are rejected before any pixel buffer is allocated.
func readHeader(data []byte) (header, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return header{}, fmt.Errorf("%w: %w", ErrNotImage, err)
	}
	if _, ok := allowedFormats[format]; !ok {
		return header{}, fmt.Errorf("%w: unsupported format %q", ErrNotImage, format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return header{}, fmt.Errorf("%w: empty image", ErrNotImage)
	}
	if cfg.Width > maxSide || cfg.Height > maxSide || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return header{}, fmt.Errorf("%w: %dx%d exceeds limits", ErrTooLarge, cfg.Width, cfg.Height)
	}
	h := header{format: format, rawW: cfg.Width, rawH: cfg.Height, orientation: 1}
	if format == "jpeg" {
		h.orientation = jpegOrientation(data)
	}
	return h, nil
}

func decodeFull(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotImage, err)
	}
	return img, nil
}

// hasAlpha reports whether any pixel is not fully opaque.
func hasAlpha(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}
	return false
}

// scaleOriented resizes src so that, after applying the EXIF orientation, the result
// is dstW x dstH. Scaling happens before the (cheap) rotation.
func scaleOriented(src image.Image, orientation, dstW, dstH int, interp draw.Interpolator) *image.RGBA {
	sw, sh := dstW, dstH
	if orientation >= 5 {
		sw, sh = dstH, dstW
	}
	// Large reductions: area-average by an integer factor first (cheap, alias-free),
	// leaving >= 2x the target for the high-quality kernel. CatmullRom straight from
	// a multi-megapixel source is ~10x slower.
	if interp == draw.CatmullRom {
		if k := min(src.Bounds().Dx()/(2*sw), src.Bounds().Dy()/(2*sh)); k >= 2 {
			if y, ok := src.(*image.YCbCr); ok {
				src = boxDownsampleYCbCr(y, k)
			} else {
				src = boxDownsample(toRGBA(src), k)
			}
		}
	}
	scaled := image.NewRGBA(image.Rect(0, 0, sw, sh))
	interp.Scale(scaled, scaled.Bounds(), src, src.Bounds(), draw.Src, nil)
	return applyOrientation(scaled, orientation)
}

func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// boxDownsample averages k x k blocks (premultiplied, so alpha-correct). Trailing
// rows/columns that do not fill a whole block are dropped (< k source pixels).
func boxDownsample(src *image.RGBA, k int) *image.RGBA {
	w, h := src.Rect.Dx()/k, src.Rect.Dy()/k
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	acc := make([]uint32, w*4)
	n := uint32(k * k)
	for dy := 0; dy < h; dy++ {
		clear(acc)
		for sy := dy * k; sy < dy*k+k; sy++ {
			row := src.Pix[sy*src.Stride : sy*src.Stride+w*k*4]
			for dx := 0; dx < w; dx++ {
				a := acc[dx*4 : dx*4+4 : dx*4+4]
				px := row[dx*k*4 : (dx+1)*k*4]
				for i := 0; i < len(px); i += 4 {
					a[0] += uint32(px[i])
					a[1] += uint32(px[i+1])
					a[2] += uint32(px[i+2])
					a[3] += uint32(px[i+3])
				}
			}
		}
		out := dst.Pix[dy*dst.Stride:]
		for i, v := range acc {
			out[i] = uint8((v + n/2) / n)
		}
	}
	return dst
}

// boxDownsampleYCbCr is boxDownsample for JPEG output, averaging the planes before
// colour conversion (luma per pixel, chroma per sample) to skip a full-size RGBA copy.
func boxDownsampleYCbCr(src *image.YCbCr, k int) *image.RGBA {
	b := src.Rect
	w, h := b.Dx()/k, b.Dy()/k
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// Chroma step (in luma pixels) between distinct chroma samples.
	csx, csy := 1, 1
	switch src.SubsampleRatio {
	case image.YCbCrSubsampleRatio422:
		csx = 2
	case image.YCbCrSubsampleRatio420:
		csx, csy = 2, 2
	case image.YCbCrSubsampleRatio440:
		csy = 2
	case image.YCbCrSubsampleRatio411:
		csx = 4
	case image.YCbCrSubsampleRatio410:
		csx, csy = 4, 2
	}
	n := uint32(k * k)
	for dy := 0; dy < h; dy++ {
		out := dst.Pix[dy*dst.Stride : dy*dst.Stride+w*4]
		y0 := b.Min.Y + dy*k
		for dx := 0; dx < w; dx++ {
			x0 := b.Min.X + dx*k
			var ys, cbs, crs, cn uint32
			for sy := y0; sy < y0+k; sy++ {
				yi := src.YOffset(x0, sy)
				for _, v := range src.Y[yi : yi+k] {
					ys += uint32(v)
				}
			}
			for sy := y0; sy < y0+k; sy += csy {
				for sx := x0; sx < x0+k; sx += csx {
					ci := src.COffset(sx, sy)
					cbs += uint32(src.Cb[ci])
					crs += uint32(src.Cr[ci])
					cn++
				}
			}
			r, g, bl := color.YCbCrToRGB(uint8((ys+n/2)/n), uint8((cbs+cn/2)/cn), uint8((crs+cn/2)/cn))
			out[dx*4], out[dx*4+1], out[dx*4+2], out[dx*4+3] = r, g, bl, 0xff
		}
	}
	return dst
}

func thumbnail(src image.Image, h header) *image.RGBA {
	w, ht := h.oriented()
	tw, th := w, ht
	if w > thumbMaxDim || ht > thumbMaxDim {
		if w >= ht {
			tw, th = thumbMaxDim, max(1, int(math.Round(float64(ht)*thumbMaxDim/float64(w))))
		} else {
			tw, th = max(1, int(math.Round(float64(w)*thumbMaxDim/float64(ht)))), thumbMaxDim
		}
	}
	return scaleOriented(src, h.orientation, tw, th, draw.ApproxBiLinear)
}

// applyOrientation returns img transformed according to the EXIF orientation tag.
func applyOrientation(img *image.RGBA, o int) *image.RGBA {
	if o < 2 || o > 8 {
		return img
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch o {
			case 2: // mirror horizontal
				sx, sy = w-1-x, y
			case 3: // rotate 180
				sx, sy = w-1-x, h-1-y
			case 4: // mirror vertical
				sx, sy = x, h-1-y
			case 5: // transpose
				sx, sy = y, x
			case 6: // rotate 90 CW
				sx, sy = y, h-1-x
			case 7: // transverse
				sx, sy = w-1-y, h-1-x
			case 8: // rotate 90 CCW
				sx, sy = w-1-y, x
			}
			si := sy*img.Stride + sx*4
			di := y*dst.Stride + x*4
			copy(dst.Pix[di:di+4], img.Pix[si:si+4])
		}
	}
	return dst
}

// jpegOrientation extracts the EXIF orientation (tag 0x0112 in IFD0) from a JPEG
// stream. It returns 1 when absent or malformed.
func jpegOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan / end of image
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(b[i+2:]))
		if segLen < 2 || i+2+segLen > len(b) {
			return 1
		}
		seg := b[i+4 : i+2+segLen]
		if marker == 0xE1 && len(seg) >= 6 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + segLen
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(t[2:]) != 42 {
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n; k++ {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) != 0x0112 {
			continue
		}
		if bo.Uint16(t[e+2:]) != 3 { // SHORT
			return 1
		}
		v := int(bo.Uint16(t[e+8:]))
		if v < 1 || v > 8 {
			return 1
		}
		return v
	}
	return 1
}

// dominantColour picks a "pleasant" dominant colour from a small thumbnail:
// a coarse 3-bit-per-channel histogram over opaque pixels, preferring pixels that
// are neither near-black, near-white nor heavily desaturated. The winning bucket's
// mean colour is returned. Falls back to all opaque pixels, then to #000000.
func dominantColour(img *image.RGBA) string {
	type bucket struct {
		n       int
		r, g, b int
	}
	var pleasant, all [512]bucket
	nPleasant, nAll := 0, 0
	bnd := img.Bounds()
	for y := 0; y < bnd.Dy(); y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < bnd.Dx(); x++ {
			r, g, b, a := unpremul(row[x*4], row[x*4+1], row[x*4+2], row[x*4+3])
			if a < 128 {
				continue
			}
			k := int(r>>5)<<6 | int(g>>5)<<3 | int(b>>5)
			add := func(bk *bucket) { bk.n++; bk.r += int(r); bk.g += int(g); bk.b += int(b) }
			add(&all[k])
			nAll++
			mx := max(r, g, b)
			mn := min(r, g, b)
			lum := (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 255
			sat := 0.0
			if mx > 0 {
				sat = float64(mx-mn) / float64(mx)
			}
			if lum > 0.08 && lum < 0.92 && sat >= 0.18 {
				add(&pleasant[k])
				nPleasant++
			}
		}
	}
	pick := func(hist *[512]bucket) string {
		best := 0
		for i := range hist {
			if hist[i].n > hist[best].n {
				best = i
			}
		}
		bk := hist[best]
		c := color.RGBA{uint8(bk.r / bk.n), uint8(bk.g / bk.n), uint8(bk.b / bk.n), 0xff}
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	// Require the pleasant subset to be at least a few percent of the image,
	// otherwise a handful of noisy pixels would dictate the colour.
	switch {
	case nPleasant > 0 && nPleasant*20 >= nAll:
		return pick(&pleasant)
	case nAll > 0:
		return pick(&all)
	default:
		return "#000000"
	}
}
