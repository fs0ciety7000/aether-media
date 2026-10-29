package mkv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---- synthetic files ----

func ebmlID(id uint64) []byte {
	switch {
	case id >= 1<<24:
		return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	case id >= 1<<16:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	case id >= 1<<8:
		return []byte{byte(id >> 8), byte(id)}
	}
	return []byte{byte(id)}
}

// el encodes an element with an 8-byte size (simple, always valid).
func el(id uint64, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	out := ebmlID(id)
	var sz [8]byte
	binary.BigEndian.PutUint64(sz[:], uint64(len(body)))
	sz[0] = 0x01 // 8-byte length marker (value < 2^56)
	out = append(out, sz[:]...)
	return append(out, body...)
}

func uintEl(id, v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return el(id, b[:])
}

func simpleBlock(track uint64, rel int16, key bool) []byte {
	flags := byte(0)
	if key {
		flags = 0x80
	}
	b := []byte{0x80 | byte(track), byte(uint16(rel) >> 8), byte(uint16(rel)), flags}
	b = append(b, make([]byte, 100)...)
	return el(idSimpleBlock, b)
}

// synth builds a file whose video keyframes are at keys (ms, one cluster
// per keyframe, one non-key frame 40 ms after each); cues lists the cue
// times. Track 2 is video, track 1 audio.
func synth(keys, cues []uint64) []byte {
	header := el(idEBML, el(0x4282, []byte("matroska")))
	info := el(idInfo, uintEl(idTimecodeScale, 1_000_000))
	tracks := el(idTracks,
		el(idTrackEntry, uintEl(idTrackNumber, 1), uintEl(idTrackType, 2)),
		el(idTrackEntry, uintEl(idTrackNumber, 2), uintEl(idTrackType, trackTypeVideo)))
	// Fixed-size SeekHead (positions patched once the layout is known).
	seekHead := func(cuesPos uint64) []byte {
		return el(idSeekHead, el(idSeek, el(idSeekID, ebmlID(idCues)), uintEl(idSeekPosition, cuesPos)))
	}
	pre := len(seekHead(0)) + len(info) + len(tracks)
	var clusters []byte
	clusterPos := map[uint64]uint64{}
	for _, k := range keys {
		clusterPos[k] = uint64(pre + len(clusters))
		clusters = append(clusters, el(idCluster, uintEl(idClusterTime, k),
			simpleBlock(1, 0, true), simpleBlock(2, 0, true), simpleBlock(2, 40, false))...)
	}
	var cps [][]byte
	for _, c := range cues {
		cps = append(cps, el(idCuePoint, uintEl(idCueTime, c),
			el(idCueTrackPos, uintEl(idCueTrack, 2), uintEl(idCueClusterPos, clusterPos[c]))))
	}
	cuesEl := el(idCues, cps...)
	seg := bytes.Join([][]byte{seekHead(uint64(pre + len(clusters))), info, tracks, clusters, cuesEl}, nil)
	return append(header, el(idSegment, seg)...)
}

func TestSyntheticIndex(t *testing.T) {
	var keys []uint64
	for i := range 200 {
		keys = append(keys, uint64(i)*1001)
	}
	data := synth(keys, keys)
	idx, err := ReadIndex(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Times) != len(keys) || idx.Times[1] != 1.001 {
		t.Fatalf("times = %d, [1] = %v", len(idx.Times), idx.Times[1])
	}
	if idx.Reads > 8 {
		t.Errorf("reads = %d, want a handful", idx.Reads)
	}
}

// seekOnly hides io.ReaderAt: the reader must fall back to Seek + Read.
type seekOnly struct{ r *bytes.Reader }

func (s seekOnly) Read(p []byte) (int, error)                { return s.r.Read(p) }
func (s seekOnly) Seek(off int64, whence int) (int64, error) { return s.r.Seek(off, whence) }

func TestSyntheticSeekOnly(t *testing.T) {
	var keys []uint64
	for i := range 20 {
		keys = append(keys, uint64(i)*2000)
	}
	data := synth(keys, keys)
	idx, err := ReadIndex(seekOnly{bytes.NewReader(data)}, int64(len(data)))
	if err != nil || len(idx.Times) != 20 {
		t.Fatalf("idx = %v, err = %v", idx, err)
	}
}

func TestSyntheticIncomplete(t *testing.T) {
	var keys, cues []uint64
	for i := range 200 {
		keys = append(keys, uint64(i)*1001)
		if i%2 == 0 { // every other keyframe only
			cues = append(cues, uint64(i)*1001)
		}
	}
	data := synth(keys, cues)
	_, err := ReadIndex(bytes.NewReader(data), int64(len(data)))
	if !errors.Is(err, ErrIncompleteCues) {
		t.Fatalf("err = %v, want ErrIncompleteCues", err)
	}
}

func TestNotMatroska(t *testing.T) {
	data := bytes.Repeat([]byte("ISO9660 "), 4096)
	if _, err := ReadIndex(bytes.NewReader(data), int64(len(data))); !errors.Is(err, ErrNotMatroska) {
		t.Fatalf("err = %v", err)
	}
}

func TestTruncated(t *testing.T) {
	var keys []uint64
	for i := range 50 {
		keys = append(keys, uint64(i)*1000)
	}
	data := synth(keys, keys)
	for _, n := range []int{10, 40, len(data) / 2, len(data) - 5} {
		if _, err := ReadIndex(bytes.NewReader(data[:n]), int64(n)); err == nil {
			t.Errorf("truncated at %d: no error", n)
		}
	}
}

// ---- real files (FFmpeg) ----

func ffmpegOrSkip(t *testing.T) (string, string) {
	t.Helper()
	ff, err1 := exec.LookPath("ffmpeg")
	fp, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("ffmpeg/ffprobe not installed")
	}
	return ff, fp
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// probeKeyframes is the reference: ffprobe's packet scan (whole file).
func probeKeyframes(t *testing.T, ffprobe, path string) []float64 {
	t.Helper()
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var kf []float64
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(strings.TrimSpace(sc.Text()), ",")
		if len(f) >= 2 && strings.HasPrefix(f[1], "K") {
			v, err := strconv.ParseFloat(f[0], 64)
			if err == nil {
				kf = append(kf, v)
			}
		}
	}
	return kf
}

func readFileIndex(t *testing.T, path string) (*Index, error) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	return ReadIndex(f, st.Size())
}

func TestFFmpegFiles(t *testing.T) {
	ff, fp := ffmpegOrSkip(t)
	dir := t.TempDir()
	cases := []struct {
		name string
		args []string
	}{
		// H.264 with B-frames, irregular GOPs (scene cuts), audio first.
		{"h264.mkv", []string{"-f", "lavfi", "-i", "sine=f=440:r=48000:d=30", "-f", "lavfi", "-i", "testsrc2=s=320x240:r=24000/1001:d=30",
			"-map", "0:a", "-map", "1:v", "-c:a", "ac3", "-c:v", "libx264", "-preset", "ultrafast", "-bf", "2", "-g", "37", "-keyint_min", "5"}},
		// HEVC-like layout via WebM / VP8.
		{"vp8.webm", []string{"-f", "lavfi", "-i", "testsrc2=s=160x120:r=25:d=20", "-c:v", "libvpx", "-g", "30", "-b:v", "200k"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(dir, c.name)
			run(t, ff, append(append([]string{"-hide_banner", "-loglevel", "error", "-y"}, c.args...), p)...)
			want := probeKeyframes(t, fp, p)
			idx, err := readFileIndex(t, p)
			if err != nil {
				if strings.Contains(err.Error(), "Unknown encoder") {
					t.Skip(err)
				}
				t.Fatal(err)
			}
			if len(idx.Times) != len(want) {
				t.Fatalf("index has %d keyframes, ffprobe %d", len(idx.Times), len(want))
			}
			for i := range want {
				if math.Abs(idx.Times[i]-want[i]) > 0.0015 {
					t.Fatalf("keyframe %d: %.4f vs ffprobe %.4f", i, idx.Times[i], want[i])
				}
			}
		})
	}
}

func TestFFmpegNoCues(t *testing.T) {
	ff, _ := ffmpegOrSkip(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "pipe.mkv")
	// A Matroska file written to a pipe has no Cues (non-seekable output).
	cmd := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=s=160x120:r=25:d=5",
		"-c:v", "libx264", "-preset", "ultrafast", "-f", "matroska", "pipe:1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readFileIndex(t, p); !errors.Is(err, ErrNoCues) {
		t.Fatalf("err = %v, want ErrNoCues", err)
	}
}

// TestBigFile checks a real remux when AETHER_MKV_SAMPLE points to one.
func TestBigFile(t *testing.T) {
	p := os.Getenv("AETHER_MKV_SAMPLE")
	if p == "" {
		t.Skip("AETHER_MKV_SAMPLE not set")
	}
	_, fp := ffmpegOrSkip(t)
	start := time.Now()
	idx, err := readFileIndex(t, p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("index: %d keyframes, %d reads, %d bytes, %s", len(idx.Times), idx.Reads, idx.Bytes, time.Since(start))
	want := probeKeyframes(t, fp, p)
	if len(want) != len(idx.Times) {
		t.Fatalf("index %d vs ffprobe %d", len(idx.Times), len(want))
	}
}
