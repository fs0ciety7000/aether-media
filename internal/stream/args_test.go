package stream

import (
	"slices"
	"strings"
	"testing"
)

// argAfter returns the value following flag (the last occurrence).
func argAfter(args []string, flag string) string {
	v := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			v = args[i+1]
		}
	}
	return v
}

func transcodeParams(hw string, d Decision, start int) jobParams {
	return jobParams{
		Input: "/media/a.mkv", Info: mkvPGS, Decision: d, HW: hw, VAAPIDevice: "/dev/dri/renderD128",
		ToneMapper: toneMapZscale, SegmentSec: 6, Dir: "/t/s/r1", Plan: uniformPlan(600, 6), StartSeg: start,
	}
}

func TestBuildArgsSoftwareTranscode(t *testing.T) {
	d := Decide(Request{Info: mkvPGS, Profile: withMax(chrome, 720, 0), AudioIndex: -1, SubtitleIndex: -1})
	args := buildArgs(transcodeParams(HWNone, d, 10))
	j := strings.Join(args, " ")
	for _, s := range []string{"-nostdin", "-hide_banner", "-loglevel error", "-ss 60.000", "-i file:/media/a.mkv",
		"-map [v]", "-map 0:1", "-c:v libx264", "-preset veryfast", "-crf 23", "-maxrate 4000000",
		"-force_key_frames expr:gte(t,60.000+n_forced*6)", "-sc_threshold 0", "-g 144", "-copyts", "-start_at_zero",
		"-avoid_negative_ts disabled", "-f hls", "-hls_time 6", "-hls_segment_type fmp4", "-hls_playlist_type vod",
		"-start_number 10", "-hls_fmp4_init_filename init.mp4", "-hls_segment_filename /t/s/r1/%d.m4s", "-c:a copy"} {
		if !strings.Contains(j, s) {
			t.Errorf("missing %q in %s", s, j)
		}
	}
	if g := argAfter(args, "-filter_complex"); g != "[0:0]scale=w=-2:h=720,format=yuv420p[v]" {
		t.Errorf("graph = %s", g)
	}
	if args[len(args)-1] != "/t/s/r1/ff.m3u8" {
		t.Errorf("output = %s", args[len(args)-1])
	}
}

func TestBuildArgsRemux(t *testing.T) {
	d := Decide(Request{Info: mkvHEVCHDR, Profile: safari, AudioIndex: -1, SubtitleIndex: -1})
	p, err := keyframePlan(keyframes{0, 2.5, 5, 7.5, 10, 12.5}, 15, 6)
	if err != nil {
		t.Fatal(err)
	}
	args := buildArgs(jobParams{Input: "/m/x.mkv", Info: mkvHEVCHDR, Decision: d, HW: HWVAAPI, SegmentSec: 6,
		Dir: "/t/a%b", Plan: p, StartSeg: 1})
	j := strings.Join(args, " ")
	for _, s := range []string{"-ss 7.700", "-map 0:0", "-map 0:1", "-c:v copy", "-tag:v hvc1", "-c:a copy",
		"-hls_time 0.000001", "-start_number 3", "-hls_segment_filename /t/a%%b/k%d.m4s"} {
		if !strings.Contains(j, s) {
			t.Errorf("missing %q in %s", s, j)
		}
	}
	if slices.Contains(args, "-hwaccel") || slices.Contains(args, "-filter_complex") {
		t.Errorf("copy must not use hwaccel or filters: %s", j)
	}
}

func TestBuildArgsHWAccel(t *testing.T) {
	d := Decide(Request{Info: mkvPGS, Profile: withMax(chrome, 720, 0), AudioIndex: -1, SubtitleIndex: -1})
	tests := []struct {
		hw      string
		encoder string
		input   []string
		graph   string
		extra   []string
	}{
		{HWVAAPI, "h264_vaapi", []string{"-hwaccel", "vaapi", "-hwaccel_device", "/dev/dri/renderD128", "-hwaccel_output_format", "vaapi"},
			"[0:0]scale_vaapi=w=-2:h=720:format=nv12[v]", []string{"-rc_mode", "-force_key_frames"}},
		{HWQSV, "h264_qsv", []string{"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"},
			"[0:0]scale_qsv=w=-1:h=720:format=nv12[v]", []string{"-look_ahead", "-g"}},
		{HWNVENC, "h264_nvenc", []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"},
			"[0:0]scale_cuda=w=-2:h=720:format=yuv420p[v]", []string{"-forced-idr", "-force_key_frames"}},
		{HWVideoToolbox, "h264_videotoolbox", []string{"-hwaccel", "videotoolbox"},
			"[0:0]scale=w=-2:h=720,format=yuv420p[v]", []string{"-allow_sw"}},
	}
	for _, tc := range tests {
		t.Run(tc.hw, func(t *testing.T) {
			args := buildArgs(transcodeParams(tc.hw, d, 0))
			if got := argAfter(args, "-c:v"); got != tc.encoder {
				t.Errorf("encoder = %s", got)
			}
			i := slices.Index(args, "-i")
			if !slices.Equal(args[i-len(tc.input):i], tc.input) {
				t.Errorf("input args = %v, want %v", args[:i], tc.input)
			}
			if g := argAfter(args, "-filter_complex"); g != tc.graph {
				t.Errorf("graph = %s, want %s", g, tc.graph)
			}
			for _, e := range tc.extra {
				if !slices.Contains(args, e) {
					t.Errorf("missing %s", e)
				}
			}
			if slices.Contains(args, "-ss") {
				t.Error("unexpected seek at segment 0")
			}
		})
	}
}

func TestBuildArgsHWToneMapFallsBackToSoftwareFilters(t *testing.T) {
	d := Decide(Request{Info: mkvHEVCHDR, Profile: withMax(chrome, 1080, 0), AudioIndex: -1, SubtitleIndex: -1})
	if !d.ToneMap {
		t.Fatal("expected tone mapping")
	}
	p := transcodeParams(HWVAAPI, d, 0)
	p.Info = mkvHEVCHDR
	args := buildArgs(p)
	g := argAfter(args, "-filter_complex")
	if !strings.Contains(g, "zscale=t=linear:npl=100") || !strings.Contains(g, "tonemap=tonemap=hable") ||
		!strings.HasSuffix(g, "scale=w=-2:h=1080,format=nv12,hwupload[v]") {
		t.Errorf("graph = %s", g)
	}
	if slices.Contains(args, "-hwaccel") || argAfter(args, "-init_hw_device") != "vaapi=va:/dev/dri/renderD128" {
		t.Errorf("hybrid path expected: %v", args)
	}
	if argAfter(args, "-c:v") != "h264_vaapi" {
		t.Error("vaapi encoder expected")
	}
	// No zscale: colorspace fallback.
	p.HW = HWNone
	p.ToneMapper = toneMapColorspace
	g = argAfter(buildArgs(p), "-filter_complex")
	if !strings.HasPrefix(g, "[0:0]colorspace=all=bt709") || strings.Contains(g, "zscale") {
		t.Errorf("fallback graph = %s", g)
	}
}

func TestBuildArgsSubtitleBurn(t *testing.T) {
	d := Decide(Request{Info: mkvPGS, Profile: chrome, AudioIndex: -1, SubtitleIndex: 2})
	g := argAfter(buildArgs(transcodeParams(HWNone, d, 0)), "-filter_complex")
	wantG := "[0:2]scale=w=1920:h=1080:force_original_aspect_ratio=decrease[sub];[0:0][sub]overlay=eof_action=pass:x=(W-w)/2:y=H-h,format=yuv420p[v]"
	if g != wantG {
		t.Errorf("pgs graph = %s", g)
	}
	d = Decide(Request{Info: mkvPGS, Profile: chrome, AudioIndex: -1, SubtitleIndex: 3, BurnSubtitle: true})
	p := transcodeParams(HWNone, d, 0)
	p.Input = "/m/it's [x].mkv"
	g = argAfter(buildArgs(p), "-filter_complex")
	if g != `[0:0]subtitles=filename=/m/it\\\'s \[x\].mkv:si=1,format=yuv420p[v]` {
		t.Errorf("text graph = %s", g)
	}
}

func TestBuildArgsAudioTranscode(t *testing.T) {
	d := Decide(Request{Info: tsMPEG2, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1})
	p := transcodeParams(HWNone, d, 0)
	p.Info = tsMPEG2
	args := buildArgs(p)
	j := strings.Join(args, " ")
	if !strings.Contains(j, "-c:a aac -ac 2 -b:a 192000") {
		t.Errorf("audio args: %s", j)
	}
	if g := argAfter(args, "-filter_complex"); !strings.HasPrefix(g, "[0:0]yadif=") {
		t.Errorf("interlaced source must be deinterlaced: %s", g)
	}
}

func TestBuildArgsURLInput(t *testing.T) {
	d := Decide(Request{Info: mkvPGS, Profile: withMax(chrome, 720, 0), AudioIndex: -1, SubtitleIndex: -1})
	p := transcodeParams(HWNone, d, 3)
	p.Input = "http://127.0.0.1:8096/internal/src/tok/movie.mkv"
	args := buildArgs(p)
	j := strings.Join(args, " ")
	wantS := "-reconnect 1 -reconnect_streamed 1 -reconnect_on_network_error 1 -reconnect_delay_max 5 -ss 18.000 -i http://127.0.0.1:8096/internal/src/tok/movie.mkv"
	if !strings.Contains(j, wantS) {
		t.Errorf("url input args: %s", j)
	}
	if strings.Contains(j, "file:http") {
		t.Error("URL must not get the file: prefix")
	}
	if inputOptions("/local/file.mkv") != nil || inputArg("/local/-x.mkv") != "file:/local/-x.mkv" {
		t.Error("local inputs")
	}
	if len(inputOptions("HTTPS://agent/x")) == 0 {
		t.Error("https input options")
	}
}
