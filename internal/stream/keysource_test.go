package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memKeyframes struct {
	mu    sync.Mutex
	m     map[string][]float64
	saves int
}

func (s *memKeyframes) LoadKeyframes(_ context.Context, id int64, v string) ([]float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kf, ok := s.m[fmt.Sprint(id, v)]
	return kf, ok
}

func (s *memKeyframes) SaveKeyframes(_ context.Context, id int64, v string, kf []float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[fmt.Sprint(id, v)] = kf
	s.saves++
}

func openLocal(path string, opens *atomic.Int64) func(context.Context) (io.ReadSeekCloser, int64, error) {
	return func(context.Context) (io.ReadSeekCloser, int64, error) {
		opens.Add(1)
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, err
		}
		return f, st.Size(), nil
	}
}

// The Matroska index gives the same keyframes as the full ffprobe scan.
func TestIndexMatchesScan(t *testing.T) {
	dir := samples(t)
	for _, name := range []string{"h264.mkv", "long.mkv"} {
		path := filepath.Join(dir, name)
		info := probeFile(t, path)
		scan, err := scanKeyframes(context.Background(), "ffprobe", path, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		m := newTestManager(t, nil)
		var opens atomic.Int64
		idx, err := m.readIndex(context.Background(), path, info, Source{Open: openLocal(path, &opens)})
		if err != nil {
			t.Fatal(err)
		}
		if len(idx) != len(scan) {
			t.Fatalf("%s: index %d keyframes, scan %d", name, len(idx), len(scan))
		}
		for i := range scan {
			if math.Abs(idx[i]-scan[i]) > 0.002 {
				t.Fatalf("%s: keyframe %d: index %.4f, scan %.4f", name, i, idx[i], scan[i])
			}
		}
	}
}

// A remote file (FFmpeg reads a URL) is remuxed from its Matroska index,
// which is stored; a later manager (restart) needs neither the index nor a
// scan.
func TestRemoteRemuxFromIndex(t *testing.T) {
	dir := samples(t)
	local := filepath.Join(dir, "h264.mkv")
	var served atomic.Int64
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served.Add(1)
		http.ServeFile(w, r, local)
	}))
	defer fileSrv.Close()
	info := probeFile(t, local)
	store := &memKeyframes{m: map[string][]float64{}}
	var opens atomic.Int64
	src := Source{MediaFileID: 42, Version: "1:2", Remote: true, Open: openLocal(local, &opens)}

	m := newTestManager(t, func(c *Config) { c.Keyframes = store })
	s, err := m.Start(context.Background(), Request{Info: info, Path: fileSrv.URL + "/h264.mkv", AudioIndex: -1, SubtitleIndex: -1,
		Profile: chrome, StartMs: 13_000, Source: src})
	if err != nil {
		t.Fatal(err)
	}
	if s.Decision.Method != MethodRemux || s.plan.kf == nil {
		t.Fatalf("decision = %+v", s.Decision)
	}
	if opens.Load() != 1 || store.saves != 1 {
		t.Fatalf("opens = %d, saves = %d", opens.Load(), store.saves)
	}
	srv := hlsServer(t, m)
	n := s.plan.segmentAt(13)
	_, pts := probeFragment(t, fetch(t, srv, s.ID, "init.mp4"), fetch(t, srv, s.ID, fmt.Sprintf("%d.m4s", n)))
	if math.Abs(pts-s.plan.starts[n]) > 0.1 {
		t.Fatalf("segment %d first pts %.3f, want %.3f", n, pts, s.plan.starts[n])
	}

	// Restart: the stored list is used, the file is not opened again.
	m2 := newTestManager(t, func(c *Config) { c.Keyframes = store })
	src.Open = func(context.Context) (io.ReadSeekCloser, int64, error) {
		return nil, 0, errors.New("must not be called")
	}
	s2, err := m2.Start(context.Background(), Request{Info: info, Path: fileSrv.URL + "/h264.mkv", AudioIndex: -1, SubtitleIndex: -1,
		Profile: chrome, Source: src})
	if err != nil || s2.Decision.Method != MethodRemux || len(s2.plan.kf) != len(s.plan.kf) {
		t.Fatalf("after restart: %v %+v", err, s2.Decision)
	}
}

// A remote file without a usable index is never read whole: the session
// falls back to a transcode at once.
func TestRemoteWithoutIndexFallsBackFast(t *testing.T) {
	dir := samples(t)
	local := filepath.Join(dir, "h264.mkv")
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, local)
	}))
	defer fileSrv.Close()
	info := probeFile(t, local)
	info.Container = "ts" // no Matroska index to read
	m := newTestManager(t, nil)
	t0 := time.Now()
	s, err := m.Start(context.Background(), Request{Info: info, Path: fileSrv.URL + "/h264.mkv", AudioIndex: -1, SubtitleIndex: -1,
		Profile: chrome, Source: Source{Remote: true}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Decision.Method != MethodTranscode || s.Decision.VideoCopy {
		t.Fatalf("decision = %+v", s.Decision)
	}
	if d := time.Since(t0); d > 5*time.Second {
		t.Fatalf("start took %s", d)
	}
}

type lockedBuf struct {
	mu sync.Mutex
	b  []byte
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b = append(l.b, p...)
	return len(p), nil
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.b)
}

// The "no zscale" warning shows only when a tone mapping is really needed,
// once per process, in French.
func TestToneMapWarningOnlyForHDR(t *testing.T) {
	dir := samples(t)
	var logs lockedBuf
	m := newTestManager(t, func(c *Config) { c.Logger = slog.New(slog.NewTextHandler(&logs, nil)) })
	m.caps()
	m.toneMapper = toneMapColorspace // as with Homebrew's FFmpeg (no libzimg)

	sdr := filepath.Join(dir, "long.mkv")
	if _, err := m.Start(context.Background(), Request{Info: probeFile(t, sdr), Path: sdr, AudioIndex: -1, SubtitleIndex: -1,
		Profile: Profile{Containers: []string{"mp4"}, VideoCodecs: []string{"vp9"}, AudioCodecs: []string{"aac"}, HLS: "mse"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "zscale") {
		t.Fatalf("warning for an SDR transcode:\n%s", logs.String())
	}
	if !hasX265 {
		t.Skip("libx265 not available: HDR half skipped")
	}
	hdr := filepath.Join(dir, "hevc10.mkv")
	for range 2 {
		s, err := m.Start(context.Background(), Request{Info: probeFile(t, hdr), Path: hdr, AudioIndex: -1, SubtitleIndex: -1, Profile: chrome})
		if err != nil {
			t.Fatal(err)
		}
		m.Stop(s.ID)
	}
	if n := strings.Count(logs.String(), "zscale"); n != 1 || !strings.Contains(logs.String(), "tone mapping HDR") {
		t.Fatalf("warnings = %d:\n%s", n, logs.String())
	}
}
