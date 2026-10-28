package probe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// TestProbeIntegration generates each sample with ffmpeg, runs the real
// ffprobe through Prober.Probe and applies the same checks as the goldens.
func TestProbeIntegration(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	p := New("")
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			path := generate(t, dir, s)
			info, err := p.Probe(context.Background(), path)
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}
			st, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			eq(t, "size", info.Size, st.Size())
			checks[s.name](t, info)
		})
	}
}

func TestProbeLeadingDashAndColon(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	src := generate(t, dir, samples[0]) // movie.mkv
	odd := filepath.Join(dir, "-weird: name.mkv")
	if err := os.Rename(src, odd); err != nil {
		t.Fatal(err)
	}
	// Probe a relative path beginning with '-' from inside dir.
	t.Chdir(dir)
	info, err := New("").Probe(context.Background(), "-weird: name.mkv")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	eq(t, "container", info.Container, "mkv")
}

func TestProbeNotMedia(t *testing.T) {
	requireFFmpeg(t)
	path := filepath.Join(t.TempDir(), "notes.mkv")
	writeFile(t, path, "this is definitely not a matroska file\n")
	_, err := New("").Probe(context.Background(), path)
	if !errors.Is(err, ErrNotMedia) {
		t.Fatalf("err = %v, want ErrNotMedia", err)
	}
}

func TestProbeMissingFile(t *testing.T) {
	_, err := New("").Probe(context.Background(), filepath.Join(t.TempDir(), "nope.mkv"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

func TestProbeBadBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.mkv")
	writeFile(t, path, "x")
	p := &Prober{FFprobe: filepath.Join(t.TempDir(), "no-such-ffprobe")}
	if _, err := p.Probe(context.Background(), path); err == nil || errors.Is(err, ErrNotMedia) {
		t.Fatalf("err = %v, want exec error", err)
	}
	if _, err := p.Version(context.Background()); err == nil {
		t.Fatal("Version with missing binary succeeded")
	}
}

func TestProbeCancelled(t *testing.T) {
	requireFFmpeg(t)
	path := filepath.Join(t.TempDir(), "a.mkv")
	writeFile(t, path, "x")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New("").Probe(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestVersion(t *testing.T) {
	requireFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, err := New("").Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^(\d+(\.\d+)*|[nN]-?\S+)$`).MatchString(v) {
		t.Errorf("Version = %q", v)
	}
}

func TestDefaults(t *testing.T) {
	p := New("")
	eq(t, "binary", p.FFprobe, "ffprobe")
	eq(t, "timeout", p.Timeout, 60*time.Second)
	var z Prober
	eq(t, "zero binary", z.binary(), "ffprobe")
	eq(t, "zero timeout", z.timeout(), 60*time.Second)
	eq(t, "custom", New("/opt/ff/ffprobe").FFprobe, "/opt/ff/ffprobe")
}
