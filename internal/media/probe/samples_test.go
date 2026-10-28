package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Run `go test ./internal/media/probe -run TestUpdateGolden -update` to
// regenerate testdata/gen_*.json from freshly generated sample media.
var updateGolden = flag.Bool("update", false, "regenerate golden ffprobe JSON in testdata")

// sample describes a tiny media file generated with ffmpeg's lavfi sources.
type sample struct {
	name string // output file name; golden is testdata/gen_<name>.json
	args func(dir string) []string
	prep func(t *testing.T, dir string)
}

const srtBody = "1\n00:00:00,000 --> 00:00:00,500\nBonjour\n"

const ffmetadata = `;FFMETADATA1
title=Sample Movie
[CHAPTER]
TIMEBASE=1/1000
START=0
END=500
title=Opening
[CHAPTER]
TIMEBASE=1/1000
START=500
END=1000
title=Ending
`

var samples = []sample{
	{
		name: "movie.mkv",
		prep: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "sub.srt"), srtBody)
			writeFile(t, filepath.Join(dir, "meta.txt"), ffmetadata)
		},
		args: func(dir string) []string {
			return []string{
				"-f", "lavfi", "-i", "testsrc=size=64x48:rate=25:duration=1",
				"-f", "lavfi", "-i", "sine=f=440:d=1",
				"-f", "lavfi", "-i", "sine=f=660:d=1",
				"-i", filepath.Join(dir, "sub.srt"),
				"-i", filepath.Join(dir, "meta.txt"),
				"-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s",
				"-map_metadata", "4", "-map_chapters", "4",
				"-c:v", "libx264", "-preset", "ultrafast", "-crf", "40", "-pix_fmt", "yuv420p",
				"-c:a", "aac", "-b:a", "32k", "-ac", "2", "-c:s", "srt",
				"-metadata:s:a:0", "language=fra", "-metadata:s:a:0", "title=Français",
				"-metadata:s:a:1", "language=en",
				"-metadata:s:s:0", "language=fre", "-metadata:s:s:0", "title=Forced",
				"-disposition:a:0", "default", "-disposition:a:1", "0", "-disposition:s:0", "forced",
			}
		},
	},
	{
		name: "hdr10.mp4",
		args: func(string) []string {
			return []string{
				"-f", "lavfi", "-i", "testsrc=size=64x48:rate=24000/1001:duration=0.5",
				"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
				"-x265-params", "log-level=error:hdr10=1:repeat-headers=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:" +
					"master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400",
				"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
				"-tag:v", "hvc1",
			}
		},
	},
	{
		name: "hlg.mkv",
		args: func(string) []string {
			return []string{
				"-f", "lavfi", "-i", "testsrc=size=64x48:rate=50:duration=0.2",
				"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
				"-x265-params", "log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc",
				"-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc",
			}
		},
	},
	{
		name: "song.mp3",
		prep: makeCover,
		args: func(dir string) []string {
			return []string{
				"-f", "lavfi", "-i", "sine=d=1",
				"-i", filepath.Join(dir, "cover.jpg"),
				"-map", "0", "-map", "1",
				"-c:a", "libmp3lame", "-b:a", "32k", "-c:v", "copy", "-disposition:v", "attached_pic",
				"-id3v2_version", "3",
				"-metadata", "title=Song", "-metadata", "artist=Artist", "-metadata", "album_artist=Album Artist",
				"-metadata", "album=Album", "-metadata", "track=3/12", "-metadata", "disc=1/2",
				"-metadata", "date=2021", "-metadata", "genre=Rock",
			}
		},
	},
	{
		name: "song.flac",
		prep: makeCover,
		args: func(dir string) []string {
			return []string{
				"-f", "lavfi", "-i", "sine=d=1:sample_rate=44100",
				"-i", filepath.Join(dir, "cover.jpg"),
				"-map", "0", "-map", "1",
				"-c:a", "flac", "-c:v", "png", "-disposition:v", "attached_pic",
				"-metadata", "TITLE=Flac Song", "-metadata", "ARTIST=Artist", "-metadata", "ALBUM=Album",
				"-metadata", "ALBUMARTIST=Various", "-metadata", "TRACKNUMBER=5", "-metadata", "DISCNUMBER=1",
				"-metadata", "DATE=2020-05-01", "-metadata", "GENRE=Jazz",
			}
		},
	},
	{
		name: "track.m4a",
		args: func(string) []string {
			return []string{
				"-f", "lavfi", "-i", "sine=d=0.5",
				"-c:a", "aac", "-b:a", "32k",
				"-metadata", "title=M4A Song", "-metadata", "artist=Artist", "-metadata", "track=2/9",
			}
		},
	},
	{
		name: "clip.webm",
		args: func(string) []string {
			return []string{
				"-f", "lavfi", "-i", "testsrc=size=64x48:rate=30:duration=0.5",
				"-f", "lavfi", "-i", "sine=d=0.5",
				"-c:v", "libvpx-vp9", "-deadline", "realtime", "-b:v", "50k", "-pix_fmt", "yuv420p",
				"-c:a", "libopus", "-b:a", "16k", "-metadata:s:a", "language=jpn",
			}
		},
	},
	{
		name: "interlaced.ts",
		args: func(string) []string {
			return []string{
				"-f", "lavfi", "-i", "testsrc=size=352x288:rate=25:duration=0.2",
				"-f", "lavfi", "-i", "sine=d=0.2",
				"-c:v", "mpeg2video", "-flags", "+ilme+ildct", "-top", "1", "-b:v", "200k",
				"-c:a", "ac3", "-b:a", "64k",
			}
		},
	},
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// makeCover writes a 32x32 cover.jpg into dir.
func makeCover(t *testing.T, dir string) {
	t.Helper()
	runFFmpeg(t, "-f", "lavfi", "-i", "color=red:s=32x32", "-frames:v", "1", filepath.Join(dir, "cover.jpg"))
}

func runFFmpeg(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-v", "error", "-y"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg %v failed (missing encoder?): %v: %s", args, err, stderr.String())
	}
}

// requireFFmpeg skips the test when ffmpeg/ffprobe are not installed.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
}

// generate creates the sample in dir and returns its path.
func generate(t *testing.T, dir string, s sample) string {
	t.Helper()
	if s.prep != nil {
		s.prep(t, dir)
	}
	out := filepath.Join(dir, s.name)
	runFFmpeg(t, append(s.args(dir), out)...)
	return out
}

// TestUpdateGolden regenerates testdata/gen_*.json when run with -update.
func TestUpdateGolden(t *testing.T) {
	if !*updateGolden {
		t.Skip("run with -update to regenerate golden files")
	}
	requireFFmpeg(t)
	dir := t.TempDir()
	for _, s := range samples {
		path := generate(t, dir, s)
		out, err := exec.Command("ffprobe", "-v", "error", "-print_format", "json",
			"-show_format", "-show_streams", "-show_chapters", path).Output()
		if err != nil {
			t.Fatalf("ffprobe %s: %v", s.name, err)
		}
		// Strip the temp directory so goldens are reproducible.
		var doc map[string]any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}
		if f, ok := doc["format"].(map[string]any); ok {
			f["filename"] = s.name
		}
		b, err := json.MarshalIndent(doc, "", "    ")
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join("testdata", "gen_"+s.name+".json"), string(b)+"\n")
	}
}
