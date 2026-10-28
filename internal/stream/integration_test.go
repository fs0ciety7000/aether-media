package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
)

// ---- sample generation -------------------------------------------------

var (
	samplesOnce sync.Once
	samplesDir  string
	samplesErr  error
	hasX265     bool
)

func TestMain(m *testing.M) {
	code := m.Run()
	if samplesDir != "" {
		os.RemoveAll(samplesDir)
	}
	os.Exit(code)
}

func needFFmpeg(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test (-short)")
	}
	for _, b := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skipf("%s not installed", b)
		}
	}
}

func ffmpegRun(args ...string) error {
	cmd := exec.Command("ffmpeg", append([]string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg %v: %w: %s", args, err, out)
	}
	return nil
}

const srtBody = "1\n00:00:01,000 --> 00:00:04,000\nHello world\n\n2\n00:00:10,000 --> 00:00:13,000\nSecond line\n"

// samples generates the test media once (shared by all tests).
func samples(t *testing.T) string {
	t.Helper()
	needFFmpeg(t)
	samplesOnce.Do(func() {
		samplesDir, samplesErr = os.MkdirTemp("", "aether-stream-test-")
		if samplesErr != nil {
			return
		}
		d := samplesDir
		srt := filepath.Join(d, "sub.srt")
		if samplesErr = os.WriteFile(srt, []byte(srtBody), 0o600); samplesErr != nil {
			return
		}
		// 20 s mkv: h264 (keyframe every 2.5 s, B-frames), 2 AAC tracks, SRT.
		samplesErr = ffmpegRun("-f", "lavfi", "-i", "testsrc2=s=320x240:r=24:d=20",
			"-f", "lavfi", "-i", "sine=f=440:d=20", "-f", "lavfi", "-i", "sine=f=880:d=20", "-i", srt,
			"-map", "0", "-map", "1", "-map", "2", "-map", "3",
			"-c:v", "libx264", "-preset", "ultrafast", "-g", "60", "-keyint_min", "60", "-bf", "2",
			"-c:a", "aac", "-b:a", "64k", "-c:s", "srt",
			"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=fre", "-metadata:s:s:0", "language=eng",
			filepath.Join(d, "h264.mkv"))
		if samplesErr != nil {
			return
		}
		// 60 s mkv for transcode / seek / throttle tests, with a bitmap (PGS) subtitle.
		sup := filepath.Join(d, "sub.sup")
		if samplesErr = os.WriteFile(sup, pgsSample(320, 240), 0o600); samplesErr != nil {
			return
		}
		samplesErr = ffmpegRun("-f", "lavfi", "-i", "testsrc2=s=320x240:r=24:d=60",
			"-f", "lavfi", "-i", "sine=f=440:d=60", "-i", sup,
			"-map", "0", "-map", "1", "-map", "2",
			"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-c:a", "aac", "-b:a", "64k", "-c:s", "copy",
			filepath.Join(d, "long.mkv"))
		if samplesErr != nil {
			return
		}
		samplesErr = ffmpegRun("-f", "lavfi", "-i", "sine=f=440:d=15", "-c:a", "flac", filepath.Join(d, "audio.flac"))
		if samplesErr != nil {
			return
		}
		samplesErr = ffmpegRun("-f", "lavfi", "-i", "testsrc2=s=320x240:r=24:d=10", "-f", "lavfi", "-i", "sine=d=10",
			"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-movflags", "+faststart", filepath.Join(d, "direct.mp4"))
		if samplesErr != nil {
			return
		}
		out, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
		if strings.Contains(string(out), "libx265") {
			hasX265 = ffmpegRun("-f", "lavfi", "-i", "testsrc2=s=320x240:r=24:d=12", "-f", "lavfi", "-i", "sine=d=12",
				"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
				"-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:hdr10=1:max-cll=1000,400",
				"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
				"-c:a", "eac3", "-b:a", "192k", filepath.Join(d, "hevc10.mkv")) == nil
		}
	})
	if samplesErr != nil {
		t.Fatal(samplesErr)
	}
	return samplesDir
}

// pgsSample builds a minimal PGS (.sup) stream: a white 64x16 box shown
// from 1 s to 4 s and from 7 s to 9 s.
func pgsSample(w, h int) []byte {
	var out []byte
	seg := func(pts uint32, typ byte, data []byte) {
		out = append(out, 'P', 'G', byte(pts>>24), byte(pts>>16), byte(pts>>8), byte(pts), 0, 0, 0, 0, typ,
			byte(len(data)>>8), byte(len(data)))
		out = append(out, data...)
	}
	u16 := func(v int) []byte { return []byte{byte(v >> 8), byte(v)} }
	cat := func(parts ...[]byte) []byte {
		var b []byte
		for _, p := range parts {
			b = append(b, p...)
		}
		return b
	}
	const bw, bh = 64, 16
	x, y := (w-bw)/2, h-bh-10
	wds := cat([]byte{1, 0}, u16(x), u16(y), u16(bw), u16(bh))
	var rle []byte
	for range bh {
		rle = append(rle, 0x00, 0xC0|byte(bw>>8), byte(bw&0xff), 0x01, 0x00, 0x00)
	}
	comp := 0
	show := func(sec float64) {
		pts := uint32(sec * 90000)
		seg(pts, 0x16, cat(u16(w), u16(h), []byte{0x10}, u16(comp), []byte{0x80, 0, 0, 1}, u16(0), []byte{0, 0}, u16(x), u16(y)))
		comp++
		seg(pts, 0x17, wds)
		seg(pts, 0x14, []byte{0, 0, 1, 235, 128, 128, 255})
		n := len(rle) + 4
		seg(pts, 0x15, cat(u16(0), []byte{0, 0xC0, byte(n >> 16), byte(n >> 8), byte(n)}, u16(bw), u16(bh), rle))
		seg(pts, 0x80, nil)
	}
	hide := func(sec float64) {
		pts := uint32(sec * 90000)
		seg(pts, 0x16, cat(u16(w), u16(h), []byte{0x10}, u16(comp), []byte{0x00, 0, 0, 0}))
		comp++
		seg(pts, 0x17, wds)
		seg(pts, 0x80, nil)
	}
	show(1)
	hide(4)
	show(7)
	hide(9)
	return out
}

func probeFile(t *testing.T, path string) *probe.Info {
	t.Helper()
	info, err := probe.New("ffprobe").Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func newTestManager(t *testing.T, mod func(*Config)) *Manager {
	t.Helper()
	cfg := Config{
		TranscodeDir:  t.TempDir(),
		MaxTranscodes: 4,
		SegmentSec:    6,
		IdleTimeout:   time.Minute,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if os.Getenv("STREAM_TEST_LOG") != "" {
		cfg.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	if mod != nil {
		mod(&cfg)
	}
	m := NewManager(cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m
}

// ---- HTTP helpers ------------------------------------------------------

func hlsServer(t *testing.T, m *Manager) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /<sid>/<name>
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		m.ServeHLS(w, r, parts[0], parts[1])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fetch(t *testing.T, srv *httptest.Server, sid, name string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/"+sid+"/"+name, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d %s", name, resp.StatusCode, body)
	}
	return body
}

var extinfRe = regexp.MustCompile(`#EXTINF:([0-9.]+),\n(\d+)\.m4s`)

// playlistSegments parses main.m3u8 into segment start times.
func playlistSegments(t *testing.T, body []byte) (starts []float64, total float64) {
	t.Helper()
	s := string(body)
	for _, tag := range []string{"#EXT-X-PLAYLIST-TYPE:VOD", "#EXT-X-INDEPENDENT-SEGMENTS", `#EXT-X-MAP:URI="init.mp4"`, "#EXT-X-ENDLIST"} {
		if !strings.Contains(s, tag) {
			t.Fatalf("playlist lacks %s:\n%s", tag, s)
		}
	}
	for i, m := range extinfRe.FindAllStringSubmatch(s, -1) {
		d, _ := strconv.ParseFloat(m[1], 64)
		if m[2] != strconv.Itoa(i) {
			t.Fatalf("segment %d named %s", i, m[2])
		}
		starts = append(starts, total)
		total += d
	}
	return starts, total
}

type probedStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	PixFmt    string `json:"pix_fmt"`
	StartTime string `json:"start_time"`
}

// probeFragment writes init+segment to a file and ffprobes it: it must be
// valid fMP4 media. Returns its streams and first video/audio pts.
func probeFragment(t *testing.T, initData, seg []byte) (streams []probedStream, firstVideo float64) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "frag.mp4")
	if err := os.WriteFile(f, append(append([]byte{}, initData...), seg...), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_streams", "-of", "json", f).Output()
	if err != nil {
		t.Fatalf("ffprobe fragment: %v", err)
	}
	var res struct {
		Streams []probedStream `json:"streams"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Streams) == 0 {
		t.Fatal("fragment has no stream")
	}
	firstVideo = math.NaN()
	// Decode-check and first frame timestamp.
	out, err = exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "frame=pts_time",
		"-read_intervals", "%+#1", "-of", "csv=p=0", f).Output()
	if err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(strings.Split(string(out), "\n")[0]), 64); err == nil {
			firstVideo = v
		}
	}
	// The whole fragment must decode without error.
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-xerror", "-i", f, "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("fragment does not decode: %v %s", err, out)
	}
	return res.Streams, firstVideo
}

func codecOf(streams []probedStream, typ string) string {
	for _, s := range streams {
		if s.CodecType == typ {
			return s.CodecName
		}
	}
	return ""
}

// ---- tests -------------------------------------------------------------

func TestIntegrationRemuxAndSeek(t *testing.T) {
	dir := samples(t)
	path := filepath.Join(dir, "h264.mkv")
	info := probeFile(t, path)
	m := newTestManager(t, nil)
	srv := hlsServer(t, m)

	// Start in the middle: the first run seeks (verified numbering).
	s, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1,
		Profile: chrome, StartMs: 13_000})
	if err != nil {
		t.Fatal(err)
	}
	if s.Decision.Method != MethodRemux || !s.Decision.VideoCopy || !s.Decision.AudioCopy {
		t.Fatalf("decision = %+v", s.Decision)
	}
	master := string(fetch(t, srv, s.ID, "master.m3u8"))
	if !strings.Contains(master, `CODECS="avc1.4d40`) || !strings.Contains(master, "mp4a.40.2") ||
		!strings.Contains(master, "RESOLUTION=320x240") || !strings.Contains(master, "main.m3u8") {
		t.Fatalf("master:\n%s", master)
	}
	starts, total := playlistSegments(t, fetch(t, srv, s.ID, "main.m3u8"))
	if math.Abs(total-float64(info.DurationMs)/1000) > 0.05 || len(starts) < 2 {
		t.Fatalf("playlist total %.3f (%d segments), duration %d ms", total, len(starts), info.DurationMs)
	}
	// Keyframes are every 2.5 s: boundaries must be on them.
	for _, st := range starts[1:] {
		if r := math.Mod(st, 2.5); r > 0.05 && r < 2.45 {
			t.Fatalf("segment start %.3f not on a keyframe", st)
		}
	}
	startSeg := -1
	for i, st := range starts {
		if math.Abs(st*1000-float64(s.StartMs)) < 1 {
			startSeg = i
		}
	}
	if startSeg <= 0 || s.StartMs > 13_000 {
		t.Fatalf("aligned StartMs = %d (starts %v)", s.StartMs, starts)
	}

	initData := fetch(t, srv, s.ID, "init.mp4")
	seg := fetch(t, srv, s.ID, fmt.Sprintf("%d.m4s", startSeg))
	streams, pts := probeFragment(t, initData, seg)
	if codecOf(streams, "video") != "h264" || codecOf(streams, "audio") != "aac" {
		t.Fatalf("streams = %+v", streams)
	}
	if math.Abs(pts-starts[startSeg]) > 0.1 {
		t.Fatalf("segment %d first pts %.3f, want ≈ %.3f", startSeg, pts, starts[startSeg])
	}
	s.mu.Lock()
	r1 := s.runs[0]
	shift, verified := r1.shift, r1.verified
	s.mu.Unlock()
	if !verified || shift != 0 {
		t.Fatalf("first run verified=%v shift=%d", verified, shift)
	}

	// Seek backwards (behind the run start): restart at segment 0.
	seg0 := fetch(t, srv, s.ID, "0.m4s")
	_, pts0 := probeFragment(t, initData, seg0)
	if pts0 > 0.1 {
		t.Fatalf("segment 0 first pts %.3f", pts0)
	}
	seg1 := fetch(t, srv, s.ID, "1.m4s")
	_, pts1 := probeFragment(t, initData, seg1)
	if math.Abs(pts1-starts[1]) > 0.1 {
		t.Fatalf("segment 1 first pts %.3f, want %.3f", pts1, starts[1])
	}
	s.mu.Lock()
	nruns := len(s.runs)
	s.mu.Unlock()
	if nruns < 2 {
		t.Fatalf("expected a restart, runs = %d", nruns)
	}
	// Every segment, concatenated, is a continuous stream.
	all := append([]byte{}, initData...)
	for i := range starts {
		all = append(all, fetch(t, srv, s.ID, fmt.Sprintf("%d.m4s", i))...)
	}
	f := filepath.Join(t.TempDir(), "all.mp4")
	os.WriteFile(f, all, 0o600)
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-count_packets",
		"-show_entries", "stream=nb_read_packets", "-of", "csv=p=0", f).Output()
	if err != nil || strings.TrimSpace(string(out)) != "480" {
		t.Fatalf("concatenated packets = %q (%v), want 480", out, err)
	}

	sdir := filepath.Join(m.cfg.TranscodeDir, s.ID)
	m.Stop(s.ID)
	if _, err := os.Stat(sdir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session dir not removed: %v", err)
	}
	if _, ok := m.Get(s.ID); ok {
		t.Fatal("session still registered")
	}
}

func TestIntegrationTranscodeSeek(t *testing.T) {
	dir := samples(t)
	path := filepath.Join(dir, "long.mkv")
	info := probeFile(t, path)
	m := newTestManager(t, func(c *Config) { c.SegmentSec = 2 })
	srv := hlsServer(t, m)

	s, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1,
		Profile: withMax(chrome, 180, 0)})
	if err != nil {
		t.Fatal(err)
	}
	d := s.Decision
	if d.Method != MethodTranscode || d.TargetHeight != 180 || d.VideoCopy {
		t.Fatalf("decision = %+v", d)
	}
	master := string(fetch(t, srv, s.ID, "master.m3u8"))
	if !strings.Contains(master, "avc1.64001e") || !strings.Contains(master, "RESOLUTION=240x180") {
		t.Fatalf("master:\n%s", master)
	}
	starts, total := playlistSegments(t, fetch(t, srv, s.ID, "main.m3u8"))
	if len(starts) != 30 || math.Abs(total-60) > 0.1 {
		t.Fatalf("segments = %d total = %.3f", len(starts), total)
	}
	initData := fetch(t, srv, s.ID, "init.mp4")
	for _, n := range []int{0, 1} {
		streams, pts := probeFragment(t, initData, fetch(t, srv, s.ID, fmt.Sprintf("%d.m4s", n)))
		if codecOf(streams, "video") != "h264" || codecOf(streams, "audio") != "aac" {
			t.Fatalf("streams = %+v", streams)
		}
		if math.Abs(pts-starts[n]) > 0.1 {
			t.Fatalf("segment %d pts %.3f", n, pts)
		}
	}
	// Seek far ahead: segment 27 (54 s) is served from a restarted (or
	// still running) process, starting at its nominal time.
	streams, pts := probeFragment(t, initData, fetch(t, srv, s.ID, "27.m4s"))
	if codecOf(streams, "video") != "h264" || math.Abs(pts-54) > 0.1 {
		t.Fatalf("segment 27: pts %.3f streams %+v", pts, streams)
	}
	// Seek back to a segment not produced by the current run.
	s.mu.Lock()
	cur := s.run
	s.mu.Unlock()
	if cur.startSeg > 0 {
		_, pts = probeFragment(t, initData, fetch(t, srv, s.ID, fmt.Sprintf("%d.m4s", cur.startSeg-1)))
		if math.Abs(pts-starts[cur.startSeg-1]) > 0.1 {
			t.Fatalf("segment %d pts %.3f", cur.startSeg-1, pts)
		}
	}
	// Restart deterministically: start a session at 40 s, then ask for 2.
	s2, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1,
		Profile: withMax(chrome, 180, 0), StartMs: 41_000})
	if err != nil {
		t.Fatal(err)
	}
	if s2.StartMs != 40_000 {
		t.Fatalf("aligned start = %d", s2.StartMs)
	}
	init2 := fetch(t, srv, s2.ID, "init.mp4")
	_, pts = probeFragment(t, init2, fetch(t, srv, s2.ID, "20.m4s"))
	if math.Abs(pts-40) > 0.1 {
		t.Fatalf("segment 20 pts %.3f", pts)
	}
	_, pts = probeFragment(t, init2, fetch(t, srv, s2.ID, "2.m4s"))
	if math.Abs(pts-4) > 0.1 {
		t.Fatalf("segment 2 pts %.3f", pts)
	}
	s2.mu.Lock()
	n := len(s2.runs)
	s2.mu.Unlock()
	if n != 2 {
		t.Fatalf("runs = %d, want 2", n)
	}
}

func TestIntegrationBurnSubtitles(t *testing.T) {
	dir := samples(t)
	m := newTestManager(t, nil)
	srv := hlsServer(t, m)

	// Bitmap (DVD) subtitle → overlay.
	path := filepath.Join(dir, "long.mkv")
	info := probeFile(t, path)
	subs := info.SubtitleStreams()
	if len(subs) != 1 || !subs[0].IsImageSub {
		t.Fatalf("subs = %+v", subs)
	}
	s, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: subs[0].Index,
		Profile: chrome, StartMs: 6000})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Decision.BurnSubtitle || s.Decision.Method != MethodTranscode {
		t.Fatalf("decision = %+v", s.Decision)
	}
	streams, pts := probeFragment(t, fetch(t, srv, s.ID, "init.mp4"), fetch(t, srv, s.ID, "1.m4s"))
	if codecOf(streams, "video") != "h264" || math.Abs(pts-6) > 0.1 {
		t.Fatalf("pgs burn: pts %.3f streams %+v", pts, streams)
	}

	// Text subtitle forced burn, from a file name that needs filter escaping.
	odd := filepath.Join(t.TempDir(), "it's [a], b; c:d\\e.mkv")
	data, err := os.ReadFile(filepath.Join(dir, "h264.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(odd, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info = probeFile(t, odd)
	sub := info.SubtitleStreams()[0]
	s, err = m.Start(context.Background(), Request{Info: info, Path: odd, AudioIndex: -1, SubtitleIndex: sub.Index,
		BurnSubtitle: true, Profile: chrome})
	if err != nil {
		t.Fatal(err)
	}
	streams, _ = probeFragment(t, fetch(t, srv, s.ID, "init.mp4"), fetch(t, srv, s.ID, "0.m4s"))
	if codecOf(streams, "video") != "h264" {
		t.Fatalf("text burn streams %+v", streams)
	}
}

func TestIntegrationToneMapHEVC10(t *testing.T) {
	dir := samples(t)
	if !hasX265 {
		t.Skip("libx265 not available")
	}
	path := filepath.Join(dir, "hevc10.mkv")
	info := probeFile(t, path)
	if v := info.Video(); v == nil || v.BitDepth != 10 || v.HDR != probe.HDR10 {
		t.Fatalf("sample video = %+v", info.Video())
	}
	m := newTestManager(t, nil)
	srv := hlsServer(t, m)
	s, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1, Profile: chrome})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Decision.ToneMap || s.Decision.Method != MethodTranscode || s.Decision.AudioCopy {
		t.Fatalf("decision = %+v", s.Decision)
	}
	streams, _ := probeFragment(t, fetch(t, srv, s.ID, "init.mp4"), fetch(t, srv, s.ID, "1.m4s"))
	if codecOf(streams, "video") != "h264" || codecOf(streams, "audio") != "aac" {
		t.Fatalf("streams %+v", streams)
	}
	for _, st := range streams {
		if st.CodecType == "video" && st.PixFmt != "yuv420p" {
			t.Fatalf("pix_fmt = %s", st.PixFmt)
		}
	}

	// Safari (HDR, hevc): remux with hvc1 copy.
	s, err = m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1, Profile: safari})
	if err != nil {
		t.Fatal(err)
	}
	if s.Decision.Method != MethodRemux {
		t.Fatalf("decision = %+v", s.Decision)
	}
	if master := string(fetch(t, srv, s.ID, "master.m3u8")); !strings.Contains(master, "hvc1.2.4.L") || !strings.Contains(master, "ec-3") || !strings.Contains(master, "VIDEO-RANGE=PQ") {
		t.Fatalf("master:\n%s", master)
	}
	streams, _ = probeFragment(t, fetch(t, srv, s.ID, "init.mp4"), fetch(t, srv, s.ID, "0.m4s"))
	if codecOf(streams, "video") != "hevc" || codecOf(streams, "audio") != "eac3" {
		t.Fatalf("streams %+v", streams)
	}
}

func TestIntegrationAudioOnlyRemux(t *testing.T) {
	dir := samples(t)
	path := filepath.Join(dir, "audio.flac")
	info := probeFile(t, path)
	m := newTestManager(t, nil)
	srv := hlsServer(t, m)
	s, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1, Profile: safari})
	if err != nil {
		t.Fatal(err)
	}
	if s.Decision.Method != MethodRemux || !s.Decision.AudioCopy {
		t.Fatalf("decision = %+v", s.Decision)
	}
	starts, _ := playlistSegments(t, fetch(t, srv, s.ID, "main.m3u8"))
	if len(starts) != 3 {
		t.Fatalf("segments = %d", len(starts))
	}
	streams, _ := probeFragment(t, fetch(t, srv, s.ID, "init.mp4"), fetch(t, srv, s.ID, "2.m4s"))
	if codecOf(streams, "audio") != "flac" {
		t.Fatalf("streams %+v", streams)
	}
}

func TestIntegrationDirectSessionAndBusy(t *testing.T) {
	dir := samples(t)
	m := newTestManager(t, func(c *Config) { c.MaxTranscodes = 1 })
	direct := probeFile(t, filepath.Join(dir, "direct.mp4"))
	mkv := probeFile(t, filepath.Join(dir, "h264.mkv"))

	d1, err := m.Start(context.Background(), Request{Info: direct, Path: filepath.Join(dir, "direct.mp4"), AudioIndex: -1, SubtitleIndex: -1, Profile: chrome, StartMs: 4321})
	if err != nil {
		t.Fatal(err)
	}
	if d1.Decision.Method != MethodDirect || d1.StartMs != 4321 {
		t.Fatalf("direct = %+v", d1)
	}
	if !m.Ping(d1.ID) {
		t.Fatal("ping direct session")
	}
	rec := httptest.NewRecorder()
	m.ServeHLS(rec, httptest.NewRequest(http.MethodGet, "/", nil), d1.ID, "main.m3u8")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("direct session HLS = %d", rec.Code)
	}

	req := Request{Info: mkv, Path: filepath.Join(dir, "h264.mkv"), AudioIndex: -1, SubtitleIndex: -1, Profile: chrome}
	s1, err := m.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), req); !errors.Is(err, ErrBusy) {
		t.Fatalf("second transcode err = %v, want ErrBusy", err)
	}
	// Direct play does not take a slot.
	if _, err := m.Start(context.Background(), Request{Info: direct, Path: "x", AudioIndex: -1, SubtitleIndex: -1, Profile: chrome}); err != nil {
		t.Fatal(err)
	}
	m.Stop(s1.ID)
	s2, err := m.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("after stop: %v", err)
	}
	if m.Ping("nope") {
		t.Fatal("ping unknown session")
	}
	rec = httptest.NewRecorder()
	m.ServeHLS(rec, httptest.NewRequest(http.MethodGet, "/", nil), s2.ID, "999.m4s")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("out of range segment = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	m.ServeHLS(rec, httptest.NewRequest(http.MethodGet, "/", nil), s2.ID, "../x")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad name = %d", rec.Code)
	}
}

func TestIntegrationIdleTimeout(t *testing.T) {
	dir := samples(t)
	path := filepath.Join(dir, "long.mkv")
	info := probeFile(t, path)
	m := newTestManager(t, func(c *Config) { c.IdleTimeout = 400 * time.Millisecond })
	s, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1, Profile: chrome})
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	r := s.run
	s.mu.Unlock()
	// Pings keep it alive.
	for range 4 {
		time.Sleep(200 * time.Millisecond)
		if !m.Ping(s.ID) {
			t.Fatal("session reaped despite pings")
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := m.Get(s.ID); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("idle session not reaped")
		}
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		t.Fatal("ffmpeg still running after idle stop")
	}
	for _, err := os.Stat(filepath.Join(m.cfg.TranscodeDir, s.ID)); !errors.Is(err, os.ErrNotExist); _, err = os.Stat(filepath.Join(m.cfg.TranscodeDir, s.ID)) {
		if time.Now().After(deadline) {
			t.Fatalf("session dir not removed: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The slot is free again.
	m.mu.Lock()
	active := m.active
	m.mu.Unlock()
	if active != 0 {
		t.Fatalf("active = %d", active)
	}
}

func TestIntegrationThrottle(t *testing.T) {
	dir := samples(t)
	path := filepath.Join(dir, "long.mkv")
	info := probeFile(t, path)
	m := newTestManager(t, func(c *Config) { c.SegmentSec = 1 })
	m.throttleAhead = 4 * time.Second
	srv := hlsServer(t, m)
	s, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1, Profile: withMax(chrome, 180, 0)})
	if err != nil {
		t.Fatal(err)
	}
	fetch(t, srv, s.ID, "0.m4s")
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		paused := s.run != nil && s.run.paused
		next := 0
		if s.run != nil {
			next = s.run.next
		}
		s.mu.Unlock()
		if paused {
			if next >= 60 {
				t.Fatalf("paused only at the end: produced %d", next)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ffmpeg never paused")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// While paused, production stops.
	countFiles := func() int {
		s.mu.Lock()
		dir := s.run.dir
		s.mu.Unlock()
		m, _ := filepath.Glob(filepath.Join(dir, "*.m4s"))
		return len(m)
	}
	before := countFiles()
	time.Sleep(time.Second)
	if after := countFiles(); after != before {
		t.Fatalf("segments produced while paused: %d → %d", before, after)
	}
	// Requesting further resumes it; the segment is produced.
	fetch(t, srv, s.ID, "8.m4s")
	s.mu.Lock()
	r := s.run
	s.mu.Unlock()
	if r.startSeg != 0 {
		t.Fatalf("unexpected restart (start %d)", r.startSeg)
	}
}

func TestIntegrationShutdown(t *testing.T) {
	dir := samples(t)
	path := filepath.Join(dir, "long.mkv")
	info := probeFile(t, path)
	root := t.TempDir()
	m := NewManager(Config{TranscodeDir: root})
	var ids []string
	for range 2 {
		s, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1, Profile: chrome})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.ID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	m.Shutdown(ctx)
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("transcode dir not cleaned: %v", entries)
	}
	for _, id := range ids {
		if _, ok := m.Get(id); ok {
			t.Fatal("session survived shutdown")
		}
	}
	if _, err := m.Start(context.Background(), Request{Info: info, Path: path, AudioIndex: -1, SubtitleIndex: -1, Profile: chrome}); err == nil {
		t.Fatal("start after shutdown")
	}
}

func TestIntegrationExtractSubtitle(t *testing.T) {
	dir := samples(t)
	out := t.TempDir()
	info := probeFile(t, filepath.Join(dir, "h264.mkv"))
	sub := info.SubtitleStreams()[0]
	for name, src := range map[string]struct {
		path  string
		index int
	}{
		"internal": {filepath.Join(dir, "h264.mkv"), sub.Index},
		"external": {filepath.Join(dir, "sub.srt"), 0},
	} {
		t.Run(name, func(t *testing.T) {
			dst := filepath.Join(out, name+".vtt")
			if err := ExtractSubtitle(context.Background(), "ffmpeg", src.path, src.index, dst); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			// Cue times are on the zero-based timeline (pts - container
			// start time, here -23 ms of AAC priming), like HLS sessions.
			if !strings.HasPrefix(s, "WEBVTT") || !regexp.MustCompile(`00:01\.0[0-4]\d --> 00:04\.0[0-4]\d`).MatchString(s) || !strings.Contains(s, "Second line") {
				t.Fatalf("vtt:\n%s", s)
			}
		})
	}
	// Errors leave no file behind.
	dst := filepath.Join(out, "bad.vtt")
	if err := ExtractSubtitle(context.Background(), "ffmpeg", filepath.Join(dir, "h264.mkv"), 0, dst); err == nil {
		t.Fatal("extracting a video stream as subtitles should fail")
	}
	if entries, _ := os.ReadDir(out); len(entries) != 2 {
		t.Fatalf("leftover files: %v", entries)
	}
}

// TestIntegrationURLInput plays a file served over HTTP (Range support, as
// remote sources will be): keyframe scan, seeking remux, subtitle extraction.
func TestIntegrationURLInput(t *testing.T) {
	dir := samples(t)
	local := filepath.Join(dir, "h264.mkv")
	fs := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer fs.Close()
	url := fs.URL + "/h264.mkv"
	info := probeFile(t, local)
	m := newTestManager(t, nil)
	srv := hlsServer(t, m)
	s, err := m.Start(context.Background(), Request{Info: info, Path: url, AudioIndex: 2, SubtitleIndex: -1,
		Profile: chrome, StartMs: 13_000})
	if err != nil {
		t.Fatal(err)
	}
	if s.Decision.Method != MethodRemux || s.plan.kf == nil {
		t.Fatalf("decision = %+v", s.Decision)
	}
	n := s.plan.segmentAt(13)
	streams, pts := probeFragment(t, fetch(t, srv, s.ID, "init.mp4"), fetch(t, srv, s.ID, fmt.Sprintf("%d.m4s", n)))
	if codecOf(streams, "video") != "h264" || math.Abs(pts-s.plan.starts[n]) > 0.1 {
		t.Fatalf("segment %d: pts %.3f streams %+v", n, pts, streams)
	}
	dst := filepath.Join(t.TempDir(), "u.vtt")
	if err := ExtractSubtitle(context.Background(), "ffmpeg", url, info.SubtitleStreams()[0].Index, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); !strings.Contains(string(b), "Hello world") {
		t.Fatalf("vtt: %s", b)
	}
}

func TestIntegrationDetectHWAccel(t *testing.T) {
	needFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	got := DetectHWAccel(ctx, "ffmpeg")
	_, dri := os.Stat("/dev/dri")
	_, nv := os.Stat("/dev/nvidia0")
	if dri != nil && nv != nil && len(got) != 0 {
		t.Fatalf("no GPU device, yet encoders detected: %v", got)
	}
	t.Logf("hardware encoders: %v", got)
	if got := DetectHWAccel(ctx, "/nonexistent/ffmpeg"); got != nil {
		t.Fatalf("missing ffmpeg: %v", got)
	}
}
