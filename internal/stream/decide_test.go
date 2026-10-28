package stream

import (
	"reflect"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
)

var (
	chrome = Profile{
		Containers:  []string{"mp4", "webm", "m4a", "mp3", "flac", "ogg", "wav"},
		VideoCodecs: []string{"h264", "vp8", "vp9", "av1"},
		AudioCodecs: []string{"aac", "mp3", "opus", "vorbis", "flac", "pcm"},
		HLS:         "mse",
	}
	firefox = Profile{
		Containers:  []string{"mp4", "webm", "mp3", "flac", "ogg", "wav"},
		VideoCodecs: []string{"h264", "vp8", "vp9", "av1"},
		AudioCodecs: []string{"aac", "mp3", "opus", "vorbis", "flac"},
		HLS:         "mse",
	}
	safari = Profile{
		Containers:  []string{"mp4", "mov", "m4a", "mp3"},
		VideoCodecs: []string{"h264", "hevc"},
		AudioCodecs: []string{"aac", "mp3", "ac3", "eac3", "flac", "alac"},
		HDR:         true,
		HLS:         "native",
	}
)

func h264(idx, w, h int) probe.Stream {
	return probe.Stream{Index: idx, Type: probe.TypeVideo, Codec: "h264", Profile: "High", Level: 41,
		Width: w, Height: h, FrameRate: 23.976, BitDepth: 8, PixFmt: "yuv420p"}
}

func audio(idx int, codec, lang string, def bool) probe.Stream {
	return probe.Stream{Index: idx, Type: probe.TypeAudio, Codec: codec, Channels: 2, SampleRate: 48000,
		Language: lang, Default: def, Bitrate: 128_000}
}

func info(container string, br int64, streams ...probe.Stream) *probe.Info {
	return &probe.Info{Container: container, DurationMs: 600_000, Bitrate: br, Streams: streams}
}

var (
	mkvH264AAC = info("mkv", 4_000_000, h264(0, 1920, 1080), audio(1, "aac", "eng", true))
	mkvHEVCHDR = info("mkv", 25_000_000,
		probe.Stream{Index: 0, Type: probe.TypeVideo, Codec: "hevc", Profile: "Main 10", Level: 153,
			Width: 3840, Height: 2160, FrameRate: 23.976, BitDepth: 10, PixFmt: "yuv420p10le",
			HDR: probe.HDR10, ColorTransfer: "smpte2084", ColorPrimaries: "bt2020", ColorSpace: "bt2020nc"},
		probe.Stream{Index: 1, Type: probe.TypeAudio, Codec: "eac3", Channels: 6, Default: true, Bitrate: 640_000})
	mp4H264AAC  = info("mp4", 3_000_000, h264(0, 1280, 720), audio(1, "aac", "eng", true))
	mp4TwoAudio = info("mp4", 3_000_000, h264(0, 1280, 720), audio(1, "aac", "eng", true), audio(2, "aac", "fre", false))
	webmVP9     = info("webm", 2_000_000,
		probe.Stream{Index: 0, Type: probe.TypeVideo, Codec: "vp9", Width: 1920, Height: 1080, BitDepth: 8, FrameRate: 30},
		audio(1, "opus", "", true))
	aviXvid = info("avi", 1_500_000,
		probe.Stream{Index: 0, Type: probe.TypeVideo, Codec: "mpeg4", Profile: "Advanced Simple Profile", Width: 720, Height: 400, BitDepth: 8, FrameRate: 25},
		audio(1, "mp3", "", false))
	mkvPGS = info("mkv", 5_000_000, h264(0, 1920, 1080), audio(1, "aac", "eng", true),
		probe.Stream{Index: 2, Type: probe.TypeSubtitle, Codec: "hdmv_pgs_subtitle", IsImageSub: true, Language: "fre"},
		probe.Stream{Index: 3, Type: probe.TypeSubtitle, Codec: "subrip", Language: "eng"})
	tsMPEG2 = info("ts", 8_000_000,
		probe.Stream{Index: 0, Type: probe.TypeVideo, Codec: "mpeg2video", Width: 720, Height: 576, BitDepth: 8, FrameRate: 25, Interlaced: true},
		audio(1, "ac3", "fre", false))
	flacOnly = info("flac", 900_000, audio(0, "flac", "", false))
	mp4Fat   = info("mp4", 40_000_000, h264(0, 1920, 1080), audio(1, "aac", "eng", true))
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name    string
		req     Request
		method  Method
		reasons []string
		check   func(t *testing.T, d Decision)
	}{
		{"chrome mkv h264/aac", Request{Info: mkvH264AAC, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1},
			MethodRemux, []string{ReasonContainer}, func(t *testing.T, d Decision) {
				want(t, d.VideoCopy && d.AudioCopy && d.AudioIndex == 1, "copy both, audio 1: %+v", d)
			}},
		{"safari mkv h264/aac", Request{Info: mkvH264AAC, Profile: safari, AudioIndex: -1, SubtitleIndex: -1},
			MethodRemux, []string{ReasonContainer}, nil},
		{"chrome mkv hevc hdr10 eac3", Request{Info: mkvHEVCHDR, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonContainer, ReasonVideoCodec, ReasonHDR, ReasonAudioCodec}, func(t *testing.T, d Decision) {
				want(t, d.ToneMap && !d.VideoCopy && !d.AudioCopy, "tone map, transcode both: %+v", d)
				want(t, d.TargetHeight == 0 && d.VideoBitrate == 20_000_000, "4K ladder: %+v", d)
			}},
		{"firefox mkv hevc hdr10 1080 cap", Request{Info: mkvHEVCHDR, Profile: withMax(firefox, 1080, 0), AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonContainer, ReasonVideoCodec, ReasonHDR, ReasonResolution, ReasonAudioCodec}, func(t *testing.T, d Decision) {
				want(t, d.TargetHeight == 1080 && d.VideoBitrate == 8_000_000 && d.ToneMap, "1080p: %+v", d)
			}},
		{"safari mkv hevc hdr10 eac3", Request{Info: mkvHEVCHDR, Profile: safari, AudioIndex: -1, SubtitleIndex: -1},
			MethodRemux, []string{ReasonContainer}, func(t *testing.T, d Decision) {
				want(t, d.VideoCopy && d.AudioCopy && !d.ToneMap, "copy hevc + eac3: %+v", d)
			}},
		{"safari hevc hdr sdr display", Request{Info: mkvHEVCHDR, Profile: noHDR(safari), AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonContainer, ReasonHDR}, func(t *testing.T, d Decision) {
				want(t, d.ToneMap && d.AudioCopy, "tone map, eac3 copy: %+v", d)
			}},
		{"chrome mp4 h264/aac", Request{Info: mp4H264AAC, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1},
			MethodDirect, nil, func(t *testing.T, d Decision) {
				want(t, d.VideoCopy && d.AudioCopy, "direct copies: %+v", d)
			}},
		{"firefox mp4 h264/aac", Request{Info: mp4H264AAC, Profile: firefox, AudioIndex: -1, SubtitleIndex: -1}, MethodDirect, nil, nil},
		{"safari mp4 h264/aac", Request{Info: mp4H264AAC, Profile: safari, AudioIndex: -1, SubtitleIndex: -1}, MethodDirect, nil, nil},
		{"mp4 second audio track", Request{Info: mp4TwoAudio, Profile: chrome, AudioIndex: 2, SubtitleIndex: -1},
			MethodRemux, []string{ReasonAudioTrack}, func(t *testing.T, d Decision) {
				want(t, d.AudioIndex == 2 && d.AudioCopy && d.VideoCopy, "audio 2 copied: %+v", d)
			}},
		{"mp4 preferred language", Request{Info: mp4TwoAudio, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1, Language: "fre"},
			MethodRemux, []string{ReasonAudioTrack}, func(t *testing.T, d Decision) {
				want(t, d.AudioIndex == 2, "fre track: %+v", d)
			}},
		{"mp4 first track explicit", Request{Info: mp4TwoAudio, Profile: chrome, AudioIndex: 1, SubtitleIndex: -1}, MethodDirect, nil, nil},
		{"chrome webm vp9/opus", Request{Info: webmVP9, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1}, MethodDirect, nil, nil},
		{"firefox webm vp9/opus", Request{Info: webmVP9, Profile: firefox, AudioIndex: -1, SubtitleIndex: -1}, MethodDirect, nil, nil},
		{"safari webm vp9/opus", Request{Info: webmVP9, Profile: safari, AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonContainer, ReasonVideoCodec, ReasonAudioCodec}, func(t *testing.T, d Decision) {
				want(t, !d.AudioCopy && !d.VideoCopy && d.VideoBitrate == 8_000_000, "%+v", d)
			}},
		{"chrome webm vp9 + second audio (vp9 copy to fmp4, MSE)", Request{Info: info("webm", 2_000_000, webmVP9.Streams[0], audio(1, "opus", "eng", true), audio(2, "opus", "fre", false)),
			Profile: chrome, AudioIndex: 2, SubtitleIndex: -1}, MethodRemux, []string{ReasonAudioTrack}, nil},
		{"chrome avi xvid/mp3", Request{Info: aviXvid, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonContainer, ReasonVideoCodec}, func(t *testing.T, d Decision) {
				want(t, d.AudioCopy && d.AudioIndex == 1 && d.TargetHeight == 0 && d.VideoBitrate == 1_500_000, "mp3 copied: %+v", d)
			}},
		{"chrome mkv h264 + PGS", Request{Info: mkvPGS, Profile: chrome, AudioIndex: -1, SubtitleIndex: 2},
			MethodTranscode, []string{ReasonContainer, ReasonSubtitleBurn}, func(t *testing.T, d Decision) {
				want(t, d.BurnSubtitle && d.SubtitleIndex == 2 && !d.VideoCopy && d.AudioCopy, "burn PGS: %+v", d)
			}},
		{"chrome mkv h264 + srt (side-car)", Request{Info: mkvPGS, Profile: chrome, AudioIndex: -1, SubtitleIndex: 3},
			MethodRemux, []string{ReasonContainer}, func(t *testing.T, d Decision) {
				want(t, !d.BurnSubtitle && d.SubtitleIndex == 3, "vtt side-car: %+v", d)
			}},
		{"chrome mkv h264 + srt forced burn", Request{Info: mkvPGS, Profile: chrome, AudioIndex: -1, SubtitleIndex: 3, BurnSubtitle: true},
			MethodTranscode, []string{ReasonContainer, ReasonSubtitleBurn}, nil},
		{"mp4 + bad subtitle index ignored", Request{Info: mp4H264AAC, Profile: chrome, AudioIndex: -1, SubtitleIndex: 7}, MethodDirect, nil, func(t *testing.T, d Decision) {
			want(t, d.SubtitleIndex == -1, "%+v", d)
		}},
		{"chrome ts mpeg2/ac3", Request{Info: tsMPEG2, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonContainer, ReasonVideoCodec, ReasonAudioCodec}, func(t *testing.T, d Decision) {
				want(t, !d.AudioCopy, "ac3 → aac: %+v", d)
			}},
		{"safari ts mpeg2/ac3", Request{Info: tsMPEG2, Profile: safari, AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonContainer, ReasonVideoCodec}, func(t *testing.T, d Decision) {
				want(t, d.AudioCopy, "ac3 copied: %+v", d)
			}},
		{"chrome flac", Request{Info: flacOnly, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1}, MethodDirect, nil, nil},
		{"safari flac (audio-only remux)", Request{Info: flacOnly, Profile: safari, AudioIndex: -1, SubtitleIndex: -1},
			MethodRemux, []string{ReasonContainer}, func(t *testing.T, d Decision) {
				want(t, d.AudioCopy && !d.VideoCopy, "%+v", d)
			}},
		{"no-flac client (audio-only transcode)", Request{Info: flacOnly, Profile: Profile{Containers: []string{"mp4"}, AudioCodecs: []string{"aac"}}, AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonContainer, ReasonAudioCodec}, func(t *testing.T, d Decision) {
				want(t, !d.AudioCopy && d.AudioIndex == 0, "%+v", d)
			}},
		{"high bitrate vs MaxBitrate", Request{Info: mp4Fat, Profile: withMax(chrome, 0, 8_000_000), AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonBitrate}, func(t *testing.T, d Decision) {
				want(t, d.VideoBitrate == 8_000_000-128_000 && d.TargetHeight == 0 && d.AudioCopy, "%+v", d)
			}},
		{"low MaxBitrate scales down", Request{Info: mp4Fat, Profile: withMax(chrome, 0, 1_500_000), AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonBitrate}, func(t *testing.T, d Decision) {
				want(t, d.TargetHeight == 480 && d.VideoBitrate == 1_500_000-128_000, "%+v", d)
			}},
		{"bitrate under MaxBitrate", Request{Info: mp4H264AAC, Profile: withMax(chrome, 0, 8_000_000), AudioIndex: -1, SubtitleIndex: -1}, MethodDirect, nil, nil},
		{"MaxHeight", Request{Info: mp4Fat, Profile: withMax(chrome, 720, 0), AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonResolution}, func(t *testing.T, d Decision) {
				want(t, d.TargetHeight == 720 && d.VideoBitrate == 4_000_000, "%+v", d)
			}},
		{"h264 10-bit", Request{Info: info("mp4", 3_000_000, probe.Stream{Index: 0, Type: probe.TypeVideo, Codec: "h264", Profile: "High 10", Width: 1280, Height: 720, BitDepth: 10}, audio(1, "aac", "", true)),
			Profile: chrome, AudioIndex: -1, SubtitleIndex: -1}, MethodTranscode, []string{ReasonVideoCodec}, nil},
		{"safari dolby vision 8.1", Request{Info: info("mp4", 15_000_000, probe.Stream{Index: 0, Type: probe.TypeVideo, Codec: "hevc", Width: 3840, Height: 2160, BitDepth: 10,
			HDR: probe.DOVI, DoviProfile: 8, DoviCompatID: 1, ColorTransfer: "smpte2084"}, audio(1, "eac3", "", true)), Profile: safari, AudioIndex: -1, SubtitleIndex: -1},
			MethodDirect, nil, nil},
		{"safari dolby vision 5", Request{Info: info("mp4", 15_000_000, probe.Stream{Index: 0, Type: probe.TypeVideo, Codec: "hevc", Width: 3840, Height: 2160, BitDepth: 10,
			HDR: probe.DOVI, DoviProfile: 5}, audio(1, "eac3", "", true)), Profile: safari, AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonHDR}, func(t *testing.T, d Decision) {
				want(t, d.ToneMap, "%+v", d)
			}},
		{"chrome dolby vision 8.4 (SDR-compatible base, hevc unsupported)", Request{Info: info("mp4", 15_000_000, probe.Stream{Index: 0, Type: probe.TypeVideo, Codec: "hevc", Width: 1920, Height: 1080, BitDepth: 10,
			HDR: probe.DOVI, DoviProfile: 8, DoviCompatID: 2}, audio(1, "aac", "", true)), Profile: chrome, AudioIndex: -1, SubtitleIndex: -1},
			MethodTranscode, []string{ReasonVideoCodec}, func(t *testing.T, d Decision) {
				want(t, !d.ToneMap, "%+v", d)
			}},
		{"nil info", Request{Profile: chrome, AudioIndex: -1, SubtitleIndex: -1}, MethodTranscode, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Decide(tc.req)
			if d.Method != tc.method {
				t.Fatalf("method = %s, want %s (%+v)", d.Method, tc.method, d)
			}
			if !reflect.DeepEqual(d.Reasons, tc.reasons) {
				t.Fatalf("reasons = %v, want %v", d.Reasons, tc.reasons)
			}
			if tc.check != nil {
				tc.check(t, d)
			}
		})
	}
}

func want(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Errorf(format, args...)
	}
}

func withMax(p Profile, h int, br int64) Profile {
	p.MaxHeight, p.MaxBitrate = h, br
	return p
}

func noHDR(p Profile) Profile {
	p.HDR = false
	return p
}

func TestKeyframePlan(t *testing.T) {
	kf := keyframes{0.02, 2.5, 5, 7.5, 7.6, 10, 12.5, 15, 17.5, 19.9}
	p, err := keyframePlan(kf, 20, 6)
	if err != nil {
		t.Fatal(err)
	}
	// 7.5 is skipped (next keyframe 0.1 s later), 19.9 too close to the end.
	wantStarts := []float64{0, 7.6, 12.5} // 17.5 < 18 (cumulative grid)
	if !reflect.DeepEqual(p.starts, wantStarts) {
		t.Fatalf("starts = %v, want %v", p.starts, wantStarts)
	}
	if !reflect.DeepEqual(p.chunkStart, []int{0, 4, 6, 10}) {
		t.Fatalf("chunkStart = %v", p.chunkStart)
	}
	if d := p.duration(2); d != 7.5 {
		t.Fatalf("last duration = %v", d)
	}
	if n := p.segmentAt(13); n != 2 {
		t.Fatalf("segmentAt(13) = %d", n)
	}
	u := uniformPlan(20, 6)
	if u.count() != 4 || u.duration(3) != 2 || u.segmentAt(12) != 2 {
		t.Fatalf("uniform plan = %+v", u)
	}
}

func TestParseKeyframes(t *testing.T) {
	out := []byte("packet,0.000000,K__\npacket,0.041667,___\npacket,2.500000,K_\npacket,N/A,K__\nformat,-0.023000\n")
	kf, err := parseKeyframes(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(kf) != 2 || kf[0] != 0.023 || kf[1] != 2.523 {
		t.Fatalf("kf = %v", kf)
	}
	if _, err := parseKeyframes([]byte("format,0\n")); err == nil {
		t.Fatal("expected error without keyframes")
	}
}

func TestKeyframeCacheLRU(t *testing.T) {
	c := newKeyframeCache(2)
	c.put("a", keyframes{1})
	c.put("b", keyframes{2})
	c.get("a")
	c.put("c", keyframes{3})
	if _, ok := c.get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if _, ok := c.get("a"); !ok {
		t.Fatal("a should be cached")
	}
}

func TestCodecStrings(t *testing.T) {
	d := Decide(Request{Info: mkvH264AAC, Profile: chrome, AudioIndex: -1, SubtitleIndex: -1})
	if got := videoCodecsString(mkvH264AAC, d); got != "avc1.640029" {
		t.Errorf("h264 copy = %s", got)
	}
	if got := audioCodecsString(mkvH264AAC, d); got != "mp4a.40.2" {
		t.Errorf("aac = %s", got)
	}
	d = Decide(Request{Info: mkvHEVCHDR, Profile: safari, AudioIndex: -1, SubtitleIndex: -1})
	if got := videoCodecsString(mkvHEVCHDR, d); got != "hvc1.2.4.L153.B0" {
		t.Errorf("hevc copy = %s", got)
	}
	if got := audioCodecsString(mkvHEVCHDR, d); got != "ec-3" {
		t.Errorf("eac3 = %s", got)
	}
	d = Decide(Request{Info: mkvHEVCHDR, Profile: withMax(chrome, 720, 0), AudioIndex: -1, SubtitleIndex: -1})
	if got := videoCodecsString(mkvHEVCHDR, d); got != "avc1.64001f" {
		t.Errorf("h264 720p transcode = %s", got)
	}
	if w, h := outputSize(mkvHEVCHDR, d); w != 1280 || h != 720 {
		t.Errorf("size = %dx%d", w, h)
	}
	if got := vp9String(&webmVP9.Streams[0]); got != "vp09.00.40.08" {
		t.Errorf("vp9 = %s", got)
	}
}

func TestEscapeFilter(t *testing.T) {
	in := `/m/it's: a [test], ok; \x.mkv`
	got := escapeFilterGraph(escapeFilterValue(in))
	wantS := `/m/it\\\'s\\: a \[test\]\, ok\; \\\\x.mkv`
	if got != wantS {
		t.Fatalf("escape = %s, want %s", got, wantS)
	}
	if escapeSegmentPattern("/tmp/100%/x") != "/tmp/100%%/x" {
		t.Fatal("segment pattern escape")
	}
}
