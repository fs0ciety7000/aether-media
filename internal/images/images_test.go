package images

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/netguard"
)

// ---------- helpers ----------

func newCache(t testing.TB, opts ...func(*Options)) *Cache {
	t.Helper()
	o := Options{Dir: filepath.Join(t.TempDir(), "cache")}
	for _, f := range opts {
		f(&o)
	}
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func writeTemp(t testing.TB, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func solid(w, h int, c color.Color) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// photo makes a deterministic, textured opaque image.
func photo(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8((x*7 + y*13 + (x*y)%17) % 32)
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 128 + n, 255})
		}
	}
	return img
}

func encJPEG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encPNG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encGIF(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := gif.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func decodeFile(t testing.TB, p string) (image.Image, string) {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img, format
}

func parseHex(t testing.TB, s string) [3]int {
	t.Helper()
	var r, g, b int
	if len(s) != 7 || s[0] != '#' {
		t.Fatalf("bad colour %q", s)
	}
	for i, dst := range []*int{&r, &g, &b} {
		for _, ch := range s[1+2*i : 3+2*i] {
			v := strings.IndexRune("0123456789abcdef", ch)
			if v < 0 {
				t.Fatalf("bad colour %q", s)
			}
			*dst = *dst*16 + v
		}
	}
	return [3]int{r, g, b}
}

func colourDist(a, b [3]int) float64 {
	dr, dg, db := float64(a[0]-b[0]), float64(a[1]-b[1]), float64(a[2]-b[2])
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

func countFiles(t testing.TB, dir string) (n int) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// withEXIFOrientation inserts an APP1/Exif segment carrying the orientation tag.
func withEXIFOrientation(jpg []byte, o uint16) []byte {
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0}
	entry := make([]byte, 12)
	binary.LittleEndian.PutUint16(entry[0:], 0x0112)
	binary.LittleEndian.PutUint16(entry[2:], 3)
	binary.LittleEndian.PutUint32(entry[4:], 1)
	binary.LittleEndian.PutUint16(entry[8:], o)
	tiff = append(tiff, entry...)
	tiff = append(tiff, 0, 0, 0, 0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

// pngWithHeaderDims returns a valid tiny PNG whose IHDR claims w x h.
func pngWithHeaderDims(t testing.TB, w, h uint32) []byte {
	b := encPNG(t, solid(1, 1, color.White))
	binary.BigEndian.PutUint32(b[16:], w)
	binary.BigEndian.PutUint32(b[20:], h)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

// ---------- blurhash ----------

func TestBlurhashSolidColours(t *testing.T) {
	// Spec note: the basis uses cos(pi*i*x/w) (not (x+0.5)/w), so odd components of a
	// flat image are not exactly zero; even ones are and quantise to (9,9,9) = "fQ".
	cases := []struct {
		name string
		c    color.Color
		want string
	}{
		// sizeFlag (4-1)+(3-1)*9 = 21 -> 'L'; black has no energy at all -> max '0',
		// DC "0000", every AC (9,9,9) -> "fQ".
		{"black", color.Black, "L00000" + strings.Repeat("fQ", 11)},
		// DC 0xFFFFFF -> "TSUA"; regression vector.
		{"white", color.White, "LGTSUA?bfQ?b~qoffQoffQfQfQfQ"},
		// DC 0x5EC4C0 = 6210752 = 10*83^3 + 71*83^2 + 45*83 + 28 -> "A;jS".
		{"teal", color.RGBA{0x5e, 0xc4, 0xc0, 0xff}, "L8A;jS"},
	}
	for _, tc := range cases {
		img := image.NewRGBA(image.Rect(0, 0, 32, 20))
		for i := range img.Pix {
			r, g, b, a := tc.c.RGBA()
			img.Pix[i] = []uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}[i%4]
		}
		got := encodeBlurhash(img, 4, 3)
		if !strings.HasPrefix(got, tc.want) || len(got) != 28 {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
		// Even-x/even-y components (i=2, j=0) and (i=0, j=2) must be zero -> "fQ".
		// Order is j-major: index = j*4+i; AC k starts at char 6+2*(k-1).
		for _, k := range []int{2, 8, 10} {
			if ac := got[6+2*(k-1) : 8+2*(k-1)]; ac != "fQ" {
				t.Errorf("%s: AC %d = %q, want fQ", tc.name, k, ac)
			}
		}
	}
	// Portrait: 3x4 -> sizeFlag 2+3*9 = 29 -> 'T'.
	if got := encodeBlurhash(image.NewRGBA(image.Rect(0, 0, 10, 20)), 3, 4); !strings.HasPrefix(got, "T00000") || len(got) != 28 {
		t.Errorf("portrait hash %q", got)
	}
}

func TestBase83(t *testing.T) {
	var sb strings.Builder
	encode83(&sb, 6210752, 4)
	encode83(&sb, 82, 1)
	encode83(&sb, 83, 2)
	if sb.String() != "A;jS~10" {
		t.Fatalf("got %q", sb.String())
	}
}

func TestBlurhashTexturedDeterministic(t *testing.T) {
	img := photo(64, 40)
	a := encodeBlurhash(img, 4, 3)
	b := encodeBlurhash(img, 4, 3)
	if a != b {
		t.Fatalf("non deterministic: %q vs %q", a, b)
	}
	if len(a) != 4+2*12 {
		t.Fatalf("len %d", len(a))
	}
	for _, ch := range a {
		if !strings.ContainsRune(base83Chars, ch) {
			t.Fatalf("invalid char %q in %q", ch, a)
		}
	}
	if a[0] != 'L' || a[1] == '0' {
		t.Fatalf("expected 4x3 with AC energy, got %q", a)
	}
}

func TestSRGBRoundTrip(t *testing.T) {
	for i := 0; i < 256; i++ {
		if got := linearToSRGB(srgbToLinearLUT[i]); got != i {
			t.Fatalf("%d -> %d", i, got)
		}
	}
}

// ---------- ingest ----------

func TestIngestLocalFormats(t *testing.T) {
	c := newCache(t)
	alpha := solid(40, 30, color.NRGBA{200, 50, 50, 255})
	for x := 0; x < 10; x++ {
		alpha.Set(x, 0, color.NRGBA{0, 0, 0, 0})
	}
	pal := image.NewPaletted(image.Rect(0, 0, 50, 20), palette.Plan9)
	for i := range pal.Pix {
		pal.Pix[i] = uint8(i % 200)
	}
	gray := image.NewGray(image.Rect(0, 0, 30, 60))
	for i := range gray.Pix {
		gray.Pix[i] = uint8(i)
	}
	gifImg := image.NewPaletted(image.Rect(0, 0, 16, 16), []color.Color{color.Transparent, color.RGBA{0, 200, 0, 255}})
	for i := range gifImg.Pix {
		gifImg.Pix[i] = uint8(i % 2)
	}
	webp, err := os.ReadFile("testdata/teal.webp")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		data   []byte
		format string
		w, h   int
		alpha  bool
	}{
		{"photo.jpg", encJPEG(t, photo(120, 80)), "jpeg", 120, 80, false},
		{"gray.jpg", encJPEG(t, gray), "jpeg", 30, 60, false},
		{"alpha.png", encPNG(t, alpha), "png", 40, 30, true},
		{"opaque.png", encPNG(t, solid(20, 20, color.NRGBA{1, 2, 3, 255})), "png", 20, 20, false},
		{"paletted.png", encPNG(t, pal), "png", 50, 20, false},
		{"anim.gif", encGIF(t, gifImg), "gif", 16, 16, true},
		{"teal.webp", webp, "webp", 64, 32, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := c.Ingest(context.Background(), writeTemp(t, tc.name, tc.data))
			if err != nil {
				t.Fatal(err)
			}
			if src.Format != tc.format || src.Width != tc.w || src.Height != tc.h || src.HasAlpha != tc.alpha {
				t.Fatalf("got %+v", src)
			}
			if len(src.Hash) != 64 || !validHash(src.Hash) {
				t.Fatalf("hash %q", src.Hash)
			}
			wantSuffix := filepath.Join(src.Hash[:2], src.Hash[2:4], src.Hash+"."+allowedFormats[tc.format])
			if !filepath.IsAbs(src.Path) || !strings.HasSuffix(src.Path, wantSuffix) {
				t.Fatalf("path %q, want suffix %q", src.Path, wantSuffix)
			}
			stored, err := os.ReadFile(src.Path)
			if err != nil || !bytes.Equal(stored, tc.data) {
				t.Fatalf("stored original differs (err %v)", err)
			}
			nx, ny := blurhashComponents(tc.w, tc.h)
			if len(src.Blurhash) != 4+2*nx*ny {
				t.Fatalf("blurhash %q", src.Blurhash)
			}
			wantFlag := "L"
			if tc.h > tc.w {
				wantFlag = "T"
			}
			if src.Blurhash[:1] != wantFlag {
				t.Fatalf("blurhash %q, want prefix %s", src.Blurhash, wantFlag)
			}
			parseHex(t, src.Dominant)
		})
	}
}

func TestIngestWebPDominant(t *testing.T) {
	c := newCache(t)
	src, err := c.Ingest(context.Background(), mustAbs(t, "testdata/teal.webp"))
	if err != nil {
		t.Fatal(err)
	}
	if d := colourDist(parseHex(t, src.Dominant), [3]int{0x5e, 0xc4, 0xc0}); d > 20 { // lossy VP8 (4:2:0) shifts the colour a bit
		t.Fatalf("dominant %s too far from #5ec4c0 (%.1f)", src.Dominant, d)
	}
}

func mustAbs(t testing.TB, p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestIngestDedup(t *testing.T) {
	c := newCache(t)
	data := encJPEG(t, photo(64, 64))
	a, err := c.Ingest(context.Background(), writeTemp(t, "a.jpg", data))
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Ingest(context.Background(), writeTemp(t, "copy-with-other-name.jpeg", data))
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("dedup: %+v vs %+v", a, b)
	}
	if n := countFiles(t, filepath.Join(c.dir, "originals")); n != 1 {
		t.Fatalf("%d originals stored", n)
	}
	other, err := c.Ingest(context.Background(), writeTemp(t, "b.jpg", encJPEG(t, photo(64, 65))))
	if err != nil || other.Hash == a.Hash {
		t.Fatalf("different content got same hash or err %v", err)
	}
}

func TestIngestRemote(t *testing.T) {
	pngData := encPNG(t, solid(30, 10, color.NRGBA{10, 120, 200, 255}))
	big := bytes.Repeat([]byte{0}, 4096)
	var gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/ok.png", func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	})
	mux.HandleFunc("/page.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>nope</body></html>"))
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(big) // Content-Length set
	})
	mux.HandleFunc("/big-chunked", func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i < 4; i++ {
			w.Write(big)
			w.(http.Flusher).Flush() // no Content-Length
		}
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newCache(t, func(o *Options) {
		o.UserAgent = "Aether/test"
		o.MaxBytes = 2048
		o.AllowNetworks = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	})
	ctx := context.Background()

	src, err := c.Ingest(ctx, srv.URL+"/ok.png")
	if err != nil {
		t.Fatal(err)
	}
	if src.Format != "png" || src.Width != 30 || src.Height != 10 || gotUA != "Aether/test" {
		t.Fatalf("got %+v ua %q", src, gotUA)
	}
	for path, want := range map[string]error{
		"/missing":     ErrNotFound,
		"/page.html":   ErrNotImage,
		"/big":         ErrTooLarge,
		"/big-chunked": ErrTooLarge,
	} {
		if _, err := c.Ingest(ctx, srv.URL+path); !errors.Is(err, want) {
			t.Errorf("%s: got %v want %v", path, err, want)
		}
	}
	if _, err := c.Ingest(ctx, srv.URL+"/boom"); err == nil {
		t.Error("/boom: expected error")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.Ingest(cctx, srv.URL+"/ok.png"); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled ctx: %v", err)
	}
}

// T-049: artwork URLs (NFO, providers) never reach the server's own network.
func TestIngestRefusesPrivateDestinations(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(encPNG(t, photo(4, 4)))
	}))
	defer srv.Close()
	redirect := httptest.NewServer(http.RedirectHandler(srv.URL+"/poster.png", http.StatusFound))
	defer redirect.Close()

	c := newCache(t)
	ctx := context.Background()
	for _, u := range []string{
		srv.URL + "/poster.png",
		strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) + "/poster.png",
		"http://169.254.169.254/latest/meta-data/iam/security-credentials/",
		"http://[::1]/poster.png",
		"http://10.0.0.1/poster.png",
		redirect.URL,
	} {
		if _, err := c.Ingest(ctx, u); !errors.Is(err, ErrBadSource) || !errors.Is(err, netguard.ErrBlocked) {
			t.Errorf("%s: err = %v, want ErrBadSource + ErrBlocked", u, err)
		}
	}
	if hit {
		t.Fatal("the loopback server was reached")
	}
}

func TestIngestBadSources(t *testing.T) {
	c := newCache(t, func(o *Options) { o.MaxBytes = 1 << 20 })
	ctx := context.Background()
	dir := t.TempDir()
	for _, src := range []string{"", "ftp://example.com/a.png", "file:///etc/passwd", "relative/a.png", "http://", "javascript:alert(1)", dir} {
		if _, err := c.Ingest(ctx, src); !errors.Is(err, ErrBadSource) {
			t.Errorf("%q: got %v, want ErrBadSource", src, err)
		}
	}
	if _, err := c.Ingest(ctx, filepath.Join(dir, "nope.png")); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if _, err := c.Ingest(ctx, writeTemp(t, "fake.png", []byte("\x89PNG not really"))); !errors.Is(err, ErrNotImage) {
		t.Errorf("fake: %v", err)
	}
	// Truncated JPEG: header decodes, pixel data does not.
	full := encJPEG(t, photo(200, 200))
	if _, err := c.Ingest(ctx, writeTemp(t, "trunc.jpg", full[:len(full)/3])); !errors.Is(err, ErrNotImage) {
		t.Errorf("truncated: %v", err)
	}
	if _, err := c.Ingest(ctx, writeTemp(t, "huge.bin", make([]byte, 2<<20))); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversize local: %v", err)
	}
	// Unsupported but decodable-by-nobody format (BMP magic).
	if _, err := c.Ingest(ctx, writeTemp(t, "a.bmp", []byte("BM\x00\x00\x00\x00"))); !errors.Is(err, ErrNotImage) {
		t.Errorf("bmp: %v", err)
	}
}

func TestIngestDecompressionBomb(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()
	for _, d := range [][2]uint32{{20000, 10}, {10, 12001}, {10000, 9000}} {
		data := pngWithHeaderDims(t, d[0], d[1])
		start := time.Now()
		_, err := c.Ingest(ctx, writeTemp(t, "bomb.png", data))
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("%v: got %v want ErrTooLarge", d, err)
		}
		if time.Since(start) > time.Second {
			t.Errorf("%v: rejection took too long", d)
		}
	}
}

func TestIngestEXIFOrientation(t *testing.T) {
	// 40x20: left half red, right half blue. Orientation 6 = rotate 90° CW to display,
	// so the displayed image is 20x40 with red on top.
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			c := color.RGBA{220, 20, 20, 255}
			if x >= 20 {
				c = color.RGBA{20, 20, 220, 255}
			}
			img.Set(x, y, c)
		}
	}
	data := withEXIFOrientation(encJPEG(t, img), 6)
	if o := jpegOrientation(data); o != 6 {
		t.Fatalf("orientation %d", o)
	}
	c := newCache(t)
	src, err := c.Ingest(context.Background(), writeTemp(t, "rot.jpg", data))
	if err != nil {
		t.Fatal(err)
	}
	if src.Width != 20 || src.Height != 40 || src.Blurhash[0] != 'T' {
		t.Fatalf("got %+v", src)
	}
	p, _, err := c.Variant(context.Background(), src.Hash, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := decodeFile(t, p)
	if b := out.Bounds(); b.Dx() != 20 || b.Dy() != 40 {
		t.Fatalf("variant dims %v", b)
	}
	if r, _, bl, _ := out.At(10, 5).RGBA(); r>>8 < 150 || bl>>8 > 100 {
		t.Fatalf("top should be red, got r=%d b=%d", r>>8, bl>>8)
	}
	if r, _, bl, _ := out.At(10, 35).RGBA(); bl>>8 < 150 || r>>8 > 100 {
		t.Fatalf("bottom should be blue, got r=%d b=%d", r>>8, bl>>8)
	}
}

func TestApplyOrientationAll(t *testing.T) {
	// 3x2 image with distinct pixels; check the displayed top-left source pixel per tag.
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < 6; i++ {
		src.Pix[i*4] = uint8(i) // pixel id in R: (x,y) -> y*3+x
	}
	// Expected id of displayed (0,0) per EXIF spec.
	want := map[int]uint8{1: 0, 2: 2, 3: 5, 4: 3, 5: 0, 6: 3, 7: 5, 8: 2}
	for o, id := range want {
		out := applyOrientation(src, o)
		if got := out.Pix[0]; got != id {
			t.Errorf("orientation %d: top-left id %d want %d", o, got, id)
		}
		if o >= 5 && (out.Rect.Dx() != 2 || out.Rect.Dy() != 3) {
			t.Errorf("orientation %d: dims %v", o, out.Rect)
		}
	}
}

// ---------- dominant colour ----------

func TestDominantTealWithBlackBorders(t *testing.T) {
	teal := color.RGBA{0x2a, 0x9d, 0x96, 0xff}
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			switch {
			case y < 50 || y >= 150: // letterbox: 50% of the image is black
				img.Set(x, y, color.Black)
			case x%37 == 0: // thin white lines (text-ish)
				img.Set(x, y, color.White)
			default:
				img.Set(x, y, teal)
			}
		}
	}
	c := newCache(t)
	src, err := c.Ingest(context.Background(), writeTemp(t, "teal.png", encPNG(t, img)))
	if err != nil {
		t.Fatal(err)
	}
	if d := colourDist(parseHex(t, src.Dominant), [3]int{0x2a, 0x9d, 0x96}); d > 20 {
		t.Fatalf("dominant %s, want ≈ #2a9d96 (dist %.1f)", src.Dominant, d)
	}
}

func TestDominantFallbacks(t *testing.T) {
	grey := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range grey.Pix {
		grey.Pix[i] = 0x80
		if i%4 == 3 {
			grey.Pix[i] = 0xff
		}
	}
	if got := dominantColour(grey); got != "#808080" {
		t.Errorf("grey: %s", got)
	}
	if got := dominantColour(image.NewRGBA(image.Rect(0, 0, 4, 4))); got != "#000000" {
		t.Errorf("transparent: %s", got)
	}
}

// ---------- variants ----------

func TestSnapWidth(t *testing.T) {
	cases := []struct{ w, src, want int }{
		{1, 5000, 92}, {92, 5000, 92}, {93, 5000, 154}, {300, 5000, 342}, {342, 5000, 342},
		{1921, 5000, 3840}, {9999, 5000, 3840}, {0, 5000, 5000}, {-5, 800, 800},
		{500, 400, 400}, {1280, 1000, 1000}, {50, 60, 60},
	}
	for _, tc := range cases {
		if got := snapWidth(tc.w, tc.src); got != tc.want {
			t.Errorf("snapWidth(%d,%d)=%d want %d", tc.w, tc.src, got, tc.want)
		}
	}
}

func TestVariantSizes(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()
	src, err := c.Ingest(ctx, writeTemp(t, "p.jpg", encJPEG(t, photo(1000, 500))))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ req, w, h int }{{300, 342, 171}, {342, 342, 171}, {50, 92, 46}, {2000, 1000, 500}, {0, 1000, 500}}
	for _, tc := range cases {
		p, ct, err := c.Variant(ctx, src.Hash, tc.req)
		if err != nil {
			t.Fatal(err)
		}
		if ct != "image/jpeg" || !strings.HasSuffix(p, ".jpg") {
			t.Fatalf("ct %s path %s", ct, p)
		}
		want := filepath.Join(c.dir, "variants", src.Hash[:2], src.Hash+"-"+strconv.Itoa(tc.w)+".jpg")
		if p != want {
			t.Fatalf("path %s want %s", p, want)
		}
		img, format := decodeFile(t, p)
		if format != "jpeg" || img.Bounds().Dx() != tc.w || img.Bounds().Dy() != tc.h {
			t.Fatalf("req %d: %s %v", tc.req, format, img.Bounds())
		}
	}
	if got := c.renders.Load(); got != 3 { // 342, 92, 1000
		t.Fatalf("renders %d", got)
	}
	if n := countFiles(t, filepath.Join(c.dir, "variants")); n != 3 {
		t.Fatalf("variant files %d", n)
	}
}

func TestVariantAlphaIsPNG(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()
	img := solid(400, 200, color.NRGBA{0, 0, 255, 128})
	src, err := c.Ingest(ctx, writeTemp(t, "a.png", encPNG(t, img)))
	if err != nil {
		t.Fatal(err)
	}
	p, ct, err := c.Variant(ctx, src.Hash, 154)
	if err != nil {
		t.Fatal(err)
	}
	out, format := decodeFile(t, p)
	if ct != "image/png" || format != "png" || out.Bounds().Dx() != 154 || out.Bounds().Dy() != 77 {
		t.Fatalf("ct %s format %s bounds %v", ct, format, out.Bounds())
	}
	if _, _, _, a := out.At(70, 30).RGBA(); a>>8 < 120 || a>>8 > 136 {
		t.Fatalf("alpha not preserved: %d", a>>8)
	}
	// GIF and WebP sources too.
	gsrc, err := c.Ingest(ctx, writeTemp(t, "g.gif", encGIF(t, photo(200, 100))))
	if err != nil {
		t.Fatal(err)
	}
	if _, ct, err := c.Variant(ctx, gsrc.Hash, 100); err != nil || ct != "image/jpeg" {
		t.Fatalf("gif variant ct %s err %v", ct, err)
	}
	wsrc, err := c.Ingest(ctx, mustAbs(t, "testdata/teal.webp"))
	if err != nil {
		t.Fatal(err)
	}
	if p, ct, err := c.Variant(ctx, wsrc.Hash, 500); err != nil || ct != "image/jpeg" {
		t.Fatalf("webp variant ct %s err %v", ct, err)
	} else if img, _ := decodeFile(t, p); img.Bounds().Dx() != 64 {
		t.Fatalf("webp variant must not upscale: %v", img.Bounds())
	}
}

func TestVariantConcurrent(t *testing.T) {
	c := newCache(t)
	src, err := c.Ingest(context.Background(), writeTemp(t, "p.jpg", encJPEG(t, photo(1600, 900))))
	if err != nil {
		t.Fatal(err)
	}
	const n = 48
	var wg sync.WaitGroup
	paths := make([]string, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			paths[i], _, errs[i] = c.Variant(context.Background(), src.Hash, 500)
		}(i)
	}
	close(start)
	wg.Wait()
	for i := range paths {
		if errs[i] != nil || paths[i] != paths[0] {
			t.Fatalf("%d: %s %v", i, paths[i], errs[i])
		}
	}
	if r := c.renders.Load(); r != 1 {
		t.Fatalf("rendered %d times", r)
	}
	if f := countFiles(t, filepath.Join(c.dir, "variants")); f != 1 {
		t.Fatalf("%d files in variants (temp leftovers?)", f)
	}
}

func TestVariantErrors(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()
	bad := []string{"", "../../etc/passwd", strings.Repeat("A", 64), strings.Repeat("g", 64), strings.Repeat("a", 63), strings.Repeat("a", 62) + "/.", strings.Repeat("a", 65)}
	for _, h := range bad {
		if _, _, err := c.Variant(ctx, h, 100); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("Variant(%q): %v", h, err)
		}
		if err := c.Remove(h); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("Remove(%q): %v", h, err)
		}
	}
	if _, _, err := c.Variant(ctx, strings.Repeat("ab", 32), 100); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown hash: %v", err)
	}
}

func TestRemove(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()
	src, err := c.Ingest(ctx, writeTemp(t, "p.jpg", encJPEG(t, photo(400, 300))))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []int{92, 342, 0} {
		if _, _, err := c.Variant(ctx, src.Hash, w); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Remove(src.Hash); err != nil {
		t.Fatal(err)
	}
	if n := countFiles(t, c.dir); n != 0 {
		t.Fatalf("%d files left", n)
	}
	if _, _, err := c.Variant(ctx, src.Hash, 92); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after remove: %v", err)
	}
	if err := c.Remove(src.Hash); err != nil {
		t.Fatalf("second remove: %v", err)
	}
}

// ---------- HTTP ----------

func TestServeVariant(t *testing.T) {
	c := newCache(t)
	src, err := c.Ingest(context.Background(), writeTemp(t, "p.jpg", encJPEG(t, photo(800, 400))))
	if err != nil {
		t.Fatal(err)
	}
	do := func(hash string, w int, inm string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/img", nil)
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		rec := httptest.NewRecorder()
		c.ServeVariant(rec, req, hash, w)
		return rec
	}

	rec := do(src.Hash, 342, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	etag := `"` + src.Hash + `-342"`
	h := rec.Header()
	if h.Get("Content-Type") != "image/jpeg" || h.Get("ETag") != etag ||
		h.Get("Cache-Control") != "public, max-age=31536000, immutable" || h.Get("Content-Length") == "" {
		t.Fatalf("headers %v", h)
	}
	img, _, err := image.Decode(rec.Body)
	if err != nil || img.Bounds().Dx() != 342 {
		t.Fatalf("body: %v %v", err, img)
	}

	for _, inm := range []string{etag, "W/" + etag, `"other", ` + etag, "*"} {
		rec = do(src.Hash, 342, inm)
		if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
			t.Fatalf("INM %q: status %d", inm, rec.Code)
		}
	}
	if rec = do(src.Hash, 342, `"`+src.Hash+`-500"`); rec.Code != http.StatusOK {
		t.Fatalf("stale etag: %d", rec.Code)
	}
	if rec = do("../../../etc/passwd", 342, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad hash: %d", rec.Code)
	}
	if rec = do(strings.Repeat("0", 64), 342, ""); rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "" {
		t.Fatalf("unknown: %d %v", rec.Code, rec.Header())
	}

	// HEAD works through ServeContent.
	req := httptest.NewRequest(http.MethodHead, "/img", nil)
	rr := httptest.NewRecorder()
	c.ServeVariant(rr, req, src.Hash, 92)
	if rr.Code != http.StatusOK || rr.Body.Len() != 0 {
		t.Fatalf("HEAD: %d len %d", rr.Code, rr.Body.Len())
	}
}

// ---------- performance ----------

func TestVariantTiming2000to342(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	c := newCache(t)
	src, err := c.Ingest(context.Background(), writeTemp(t, "big.jpg", encJPEG(t, photo(2000, 3000))))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, _, err := c.Variant(context.Background(), src.Hash, 342); err != nil {
		t.Fatal(err)
	}
	cold := time.Since(start)
	start = time.Now()
	if _, _, err := c.Variant(context.Background(), src.Hash, 342); err != nil {
		t.Fatal(err)
	}
	t.Logf("2000x3000 JPEG -> 342px: cold %v, cached %v", cold, time.Since(start))
}

func BenchmarkVariant342From2000JPEG(b *testing.B) {
	data := encJPEG(b, photo(2000, 3000))
	for b.Loop() {
		b.StopTimer()
		c := newCache(b)
		src, err := c.Ingest(context.Background(), writeTemp(b, "big.jpg", data))
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if _, _, err := c.Variant(context.Background(), src.Hash, 342); err != nil {
			b.Fatal(err)
		}
	}
}

func TestVariantDownscaleQuality(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()
	// 1px black/white stripes: a heavy downscale must average to grey, not alias.
	stripes := image.NewGray(image.Rect(0, 0, 2000, 200))
	for i := range stripes.Pix {
		if (i%2000)%2 == 0 {
			stripes.Pix[i] = 0xff
		}
	}
	src, err := c.Ingest(ctx, writeTemp(t, "stripes.png", encPNG(t, stripes)))
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := c.Variant(ctx, src.Hash, 92)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := decodeFile(t, p)
	for x := 2; x < 90; x += 7 {
		if r, _, _, _ := out.At(x, 5).RGBA(); r>>8 < 100 || r>>8 > 160 {
			t.Fatalf("aliasing at x=%d: %d", x, r>>8)
		}
	}
	// JPEG (YCbCr fast path) keeps its colour.
	want := color.RGBA{0x2a, 0x9d, 0x96, 0xff}
	jsrc, err := c.Ingest(ctx, writeTemp(t, "flat.jpg", encJPEG(t, solid(2000, 1000, want))))
	if err != nil {
		t.Fatal(err)
	}
	p, _, err = c.Variant(ctx, jsrc.Hash, 342)
	if err != nil {
		t.Fatal(err)
	}
	out, _ = decodeFile(t, p)
	r, g, b, _ := out.At(170, 85).RGBA()
	if d := colourDist([3]int{int(r >> 8), int(g >> 8), int(b >> 8)}, [3]int{0x2a, 0x9d, 0x96}); d > 6 {
		t.Fatalf("colour drift %.1f", d)
	}
	if out.Bounds().Dx() != 342 || out.Bounds().Dy() != 171 {
		t.Fatalf("bounds %v", out.Bounds())
	}
}
