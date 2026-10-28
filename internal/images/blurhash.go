package images

import (
	"image"
	"math"
	"strings"
)

// Blurhash encoder (https://github.com/woltapp/blurhash/blob/master/Algorithm.md).
// Only encoding is implemented: the web UI decodes the hash client-side.

const base83Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~"

// srgbToLinearLUT maps an 8-bit sRGB channel value to linear light [0,1].
var srgbToLinearLUT = func() [256]float64 {
	var t [256]float64
	for i := range t {
		v := float64(i) / 255
		if v <= 0.04045 {
			t[i] = v / 12.92
		} else {
			t[i] = math.Pow((v+0.055)/1.055, 2.4)
		}
	}
	return t
}()

func linearToSRGB(v float64) int {
	v = math.Max(0, math.Min(1, v))
	if v <= 0.0031308 {
		return int(v*12.92*255 + 0.5)
	}
	return int((1.055*math.Pow(v, 1/2.4)-0.055)*255 + 0.5)
}

func signPow(v, exp float64) float64 {
	return math.Copysign(math.Pow(math.Abs(v), exp), v)
}

func encode83(sb *strings.Builder, value, length int) {
	for i := 1; i <= length; i++ {
		div := 1
		for j := 0; j < length-i; j++ {
			div *= 83
		}
		sb.WriteByte(base83Chars[(value/div)%83])
	}
}

// blurhashComponents picks 4x3 for landscape/square images and 3x4 for portrait ones.
func blurhashComponents(w, h int) (nx, ny int) {
	if h > w {
		return 3, 4
	}
	return 4, 3
}

// encodeBlurhash computes the blurhash of img (meant to be a small thumbnail) with
// nx*ny components (each 1..9). Colours are read un-premultiplied (alpha ignored).
func encodeBlurhash(img *image.RGBA, nx, ny int) string {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 || nx < 1 || nx > 9 || ny < 1 || ny > 9 {
		return ""
	}

	// Linearise once.
	lin := make([][3]float64, w*h)
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w; x++ {
			r, g, bl, _ := unpremul(row[x*4], row[x*4+1], row[x*4+2], row[x*4+3])
			lin[y*w+x] = [3]float64{srgbToLinearLUT[r], srgbToLinearLUT[g], srgbToLinearLUT[bl]}
		}
	}
	cosX := make([]float64, w)
	cosY := make([]float64, h)

	factors := make([][3]float64, 0, nx*ny)
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			for x := 0; x < w; x++ {
				cosX[x] = math.Cos(math.Pi * float64(i) * float64(x) / float64(w))
			}
			for y := 0; y < h; y++ {
				cosY[y] = math.Cos(math.Pi * float64(j) * float64(y) / float64(h))
			}
			var r, g, bl float64
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					basis := cosX[x] * cosY[y]
					p := lin[y*w+x]
					r += basis * p[0]
					g += basis * p[1]
					bl += basis * p[2]
				}
			}
			norm := 2.0
			if i == 0 && j == 0 {
				norm = 1
			}
			scale := norm / float64(w*h)
			factors = append(factors, [3]float64{r * scale, g * scale, bl * scale})
		}
	}

	var sb strings.Builder
	sb.Grow(4 + 2*nx*ny)
	encode83(&sb, (nx-1)+(ny-1)*9, 1)

	dc, ac := factors[0], factors[1:]
	maxValue := 1.0
	if len(ac) > 0 {
		actualMax := 0.0
		for _, f := range ac {
			actualMax = math.Max(actualMax, math.Max(math.Abs(f[0]), math.Max(math.Abs(f[1]), math.Abs(f[2]))))
		}
		quantMax := int(math.Max(0, math.Min(82, math.Floor(actualMax*166-0.5))))
		maxValue = float64(quantMax+1) / 166
		encode83(&sb, quantMax, 1)
	} else {
		encode83(&sb, 0, 1)
	}

	encode83(&sb, linearToSRGB(dc[0])<<16|linearToSRGB(dc[1])<<8|linearToSRGB(dc[2]), 4)

	q := func(v float64) int {
		return int(math.Max(0, math.Min(18, math.Floor(signPow(v/maxValue, 0.5)*9+9.5))))
	}
	for _, f := range ac {
		encode83(&sb, q(f[0])*19*19+q(f[1])*19+q(f[2]), 2)
	}
	return sb.String()
}

// unpremul converts a premultiplied RGBA pixel to straight alpha.
func unpremul(r, g, b, a uint8) (uint8, uint8, uint8, uint8) {
	if a == 0xff || a == 0 {
		return r, g, b, a
	}
	f := func(c uint8) uint8 {
		v := (uint32(c)*255 + uint32(a)/2) / uint32(a)
		if v > 255 {
			v = 255
		}
		return uint8(v)
	}
	return f(r), f(g), f(b), a
}
