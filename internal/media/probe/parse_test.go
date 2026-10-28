package probe

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func loadFixture(t *testing.T, name string) *Info {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	info, err := ParseOutput(data, "")
	if err != nil {
		t.Fatalf("ParseOutput(%s): %v", name, err)
	}
	return info
}

func eq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// checks are shared by the golden tests and the live ffprobe integration test.
var checks = map[string]func(t *testing.T, i *Info){
	"movie.mkv": func(t *testing.T, i *Info) {
		eq(t, "container", i.Container, "mkv")
		eq(t, "duration", i.DurationMs, 1023)
		eq(t, "tags.title", i.Tags["title"], "Sample Movie")
		if i.Size <= 0 || i.Bitrate <= 0 {
			t.Errorf("size/bitrate = %d/%d, want > 0", i.Size, i.Bitrate)
		}
		v := i.Video()
		if v == nil {
			t.Fatal("no video")
		}
		eq(t, "v.codec", v.Codec, "h264")
		eq(t, "v.width", v.Width, 64)
		eq(t, "v.height", v.Height, 48)
		near(t, "v.fps", v.FrameRate, 25)
		eq(t, "v.bitdepth", v.BitDepth, 8)
		eq(t, "v.hdr", v.HDR, "")
		eq(t, "v.interlaced", v.Interlaced, false)
		a := i.AudioStreams()
		if len(a) != 2 {
			t.Fatalf("audio streams = %d", len(a))
		}
		eq(t, "a0.codec", a[0].Codec, "aac")
		eq(t, "a0.profile", a[0].Profile, "LC")
		eq(t, "a0.lang", a[0].Language, "fre")
		eq(t, "a0.title", a[0].Title, "Français")
		eq(t, "a0.default", a[0].Default, true)
		eq(t, "a0.channels", a[0].Channels, 2)
		eq(t, "a0.layout", a[0].ChannelLayout, "stereo")
		eq(t, "a0.rate", a[0].SampleRate, 44100)
		eq(t, "a1.lang", a[1].Language, "eng")
		eq(t, "a1.default", a[1].Default, false)
		s := i.SubtitleStreams()
		if len(s) != 1 {
			t.Fatalf("subtitle streams = %d", len(s))
		}
		eq(t, "s0.codec", s[0].Codec, "subrip")
		eq(t, "s0.lang", s[0].Language, "fre")
		eq(t, "s0.forced", s[0].Forced, true)
		eq(t, "s0.image", s[0].IsImageSub, false)
		eq(t, "s0.text", s[0].IsTextSub(), true)
		want := []Chapter{{0, 500, "Opening"}, {500, 1000, "Ending"}}
		if !reflect.DeepEqual(i.Chapters, want) {
			t.Errorf("chapters = %+v, want %+v", i.Chapters, want)
		}
	},
	"hdr10.mp4": func(t *testing.T, i *Info) {
		eq(t, "container", i.Container, "mp4")
		eq(t, "duration", i.DurationMs, 500)
		v := i.Video()
		if v == nil {
			t.Fatal("no video")
		}
		eq(t, "codec", v.Codec, "hevc")
		eq(t, "profile", v.Profile, "Main 10")
		eq(t, "bitdepth", v.BitDepth, 10)
		eq(t, "pixfmt", v.PixFmt, "yuv420p10le")
		eq(t, "hdr", v.HDR, HDR10)
		eq(t, "primaries", v.ColorPrimaries, "bt2020")
		eq(t, "transfer", v.ColorTransfer, "smpte2084")
		near(t, "fps", v.FrameRate, 23.976)
		eq(t, "lang und", v.Language, "")
		if v.Level <= 0 {
			t.Errorf("level = %d", v.Level)
		}
		eq(t, "chapters", len(i.Chapters), 0)
	},
	"hlg.mkv": func(t *testing.T, i *Info) {
		eq(t, "container", i.Container, "mkv")
		v := i.Video()
		if v == nil {
			t.Fatal("no video")
		}
		eq(t, "hdr", v.HDR, HLG)
		eq(t, "bitdepth", v.BitDepth, 10)
		near(t, "fps", v.FrameRate, 50)
		eq(t, "duration", i.DurationMs, 200)
	},
	"song.mp3": func(t *testing.T, i *Info) {
		eq(t, "container", i.Container, "mp3")
		if i.Video() != nil {
			t.Error("cover art must not be reported as video")
		}
		eq(t, "audio only", i.IsAudioOnly(), true)
		pic := i.AttachedPic()
		if pic == nil {
			t.Fatal("no attached pic")
		}
		eq(t, "pic.codec", pic.Codec, "mjpeg")
		eq(t, "pic.type", pic.Type, TypeVideo)
		eq(t, "pic.fps", pic.FrameRate, 0)
		for k, want := range map[string]string{
			"title": "Song", "artist": "Artist", "album_artist": "Album Artist", "album": "Album",
			"track": "3/12", "disc": "1/2", "date": "2021", "genre": "Rock",
		} {
			eq(t, "tags."+k, i.Tags[k], want)
		}
		a := i.AudioStreams()
		eq(t, "audio", len(a), 1)
		eq(t, "a.codec", a[0].Codec, "mp3")
		if i.DurationMs < 1000 || i.DurationMs > 1100 {
			t.Errorf("duration = %d", i.DurationMs)
		}
	},
	"song.flac": func(t *testing.T, i *Info) {
		eq(t, "container", i.Container, "flac")
		eq(t, "duration", i.DurationMs, 1000)
		if i.Video() != nil {
			t.Error("cover art must not be reported as video")
		}
		if p := i.AttachedPic(); p == nil || p.Codec != "png" {
			t.Errorf("attached pic = %+v", p)
		}
		for k, want := range map[string]string{
			"title": "Flac Song", "artist": "Artist", "album": "Album", "album_artist": "Various",
			"track": "5", "disc": "1", "date": "2020-05-01", "genre": "Jazz",
		} {
			eq(t, "tags."+k, i.Tags[k], want)
		}
		a := i.AudioStreams()[0]
		eq(t, "bitdepth", a.BitDepth, 16)
		eq(t, "rate", a.SampleRate, 44100)
		eq(t, "layout", a.ChannelLayout, "mono")
	},
	"track.m4a": func(t *testing.T, i *Info) {
		eq(t, "container", i.Container, "m4a")
		eq(t, "tags.title", i.Tags["title"], "M4A Song")
		eq(t, "tags.track", i.Tags["track"], "2/9")
		eq(t, "audio only", i.IsAudioOnly(), true)
	},
	"clip.webm": func(t *testing.T, i *Info) {
		eq(t, "container", i.Container, "webm")
		v := i.Video()
		if v == nil {
			t.Fatal("no video")
		}
		eq(t, "codec", v.Codec, "vp9")
		near(t, "fps", v.FrameRate, 30)
		a := i.AudioStreams()
		eq(t, "a.codec", a[0].Codec, "opus")
		eq(t, "a.lang", a[0].Language, "jpn")
		eq(t, "a.rate", a[0].SampleRate, 48000)
	},
	"interlaced.ts": func(t *testing.T, i *Info) {
		eq(t, "container", i.Container, "ts")
		v := i.Video()
		if v == nil {
			t.Fatal("no video")
		}
		eq(t, "codec", v.Codec, "mpeg2video")
		eq(t, "interlaced", v.Interlaced, true)
		eq(t, "width", v.Width, 352)
		eq(t, "a.codec", i.AudioStreams()[0].Codec, "ac3")
	},
}

func TestGoldenGenerated(t *testing.T) {
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			check, ok := checks[s.name]
			if !ok {
				t.Fatalf("no checks for %s", s.name)
			}
			check(t, loadFixture(t, "gen_"+s.name+".json"))
		})
	}
}

func TestGoldenDolbyVisionAtmosPGS(t *testing.T) {
	i := loadFixture(t, "hand_dovi_atmos_pgs.json")
	eq(t, "container", i.Container, "mkv")
	eq(t, "duration", i.DurationMs, 7803042)
	eq(t, "size", i.Size, 48123456789)
	eq(t, "bitrate", i.Bitrate, 49337862)
	eq(t, "tags.title", i.Tags["title"], "Dune")
	eq(t, "tags.encoder lower-cased", i.Tags["encoder"], "libebml v1.4.4 + libmatroska v1.7.1")

	v := i.Video()
	eq(t, "hdr", v.HDR, DOVI)
	eq(t, "dv profile", v.DoviProfile, 8)
	eq(t, "dv level", v.DoviLevel, 6)
	eq(t, "dv compat", v.DoviCompatID, 1)
	eq(t, "bitdepth", v.BitDepth, 10)
	eq(t, "level", v.Level, 153)
	eq(t, "bitrate from BPS tag", v.Bitrate, 45123456)
	eq(t, "width", v.Width, 3840)
	near(t, "fps", v.FrameRate, 23.976)

	a := i.AudioStreams()
	eq(t, "audio count", len(a), 2)
	eq(t, "truehd codec", a[0].Codec, "truehd")
	eq(t, "truehd atmos", a[0].IsAtmos(), true)
	eq(t, "truehd channels", a[0].Channels, 8)
	eq(t, "truehd layout", a[0].ChannelLayout, "7.1")
	eq(t, "truehd bitdepth", a[0].BitDepth, 24)
	eq(t, "truehd bitrate", a[0].Bitrate, 4012345)
	eq(t, "truehd lang", a[0].Language, "eng")
	eq(t, "eac3 atmos", a[1].IsAtmos(), true)
	eq(t, "eac3 lang", a[1].Language, "fre")
	eq(t, "eac3 bitrate", a[1].Bitrate, 768000)

	s := i.SubtitleStreams()
	eq(t, "subs", len(s), 2)
	eq(t, "pgs image", s[0].IsImageSub, true)
	eq(t, "pgs text", s[0].IsTextSub(), false)
	eq(t, "pgs forced", s[0].Forced, true)
	eq(t, "pgs lang", s[0].Language, "fre")
	eq(t, "sdh from title", s[1].HearingImpaired, true)
	eq(t, "sdh not forced", s[1].Forced, false)

	att := i.Streams[5]
	eq(t, "attachment type", att.Type, TypeAttachment)
	eq(t, "attachment filename", att.Filename, "Arial.ttf")
	eq(t, "attachment mime", att.MimeType, "application/x-truetype-font")

	want := []Chapter{{0, 600000, "Chapter 01"}, {600000, 7803042, "Chapter 02"}}
	if !reflect.DeepEqual(i.Chapters, want) {
		t.Errorf("chapters = %+v, want %+v (sorted)", i.Chapters, want)
	}
}

func TestGoldenDTSHDMAImageSubs(t *testing.T) {
	data, err := os.ReadFile("testdata/hand_dts_hdma_dvdsub.json")
	if err != nil {
		t.Fatal(err)
	}
	i, err := ParseOutput(data, "/media/S01E01.m2ts")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "container", i.Container, "ts")
	eq(t, "duration", i.DurationMs, 2650500)
	eq(t, "bitrate derived", i.Bitrate, int64(9000000000*8*1000/2650500))
	eq(t, "no tags", len(i.Tags), 0)

	v := i.Video()
	eq(t, "interlaced", v.Interlaced, true)
	near(t, "fps uses avg", v.FrameRate, 25)
	eq(t, "sdr", v.HDR, "")
	eq(t, "level", v.Level, 41)

	a := i.AudioStreams()
	eq(t, "dts profile", a[0].Profile, "DTS-HD MA")
	eq(t, "dts lang deu→ger", a[0].Language, "ger")
	eq(t, "dts bitdepth", a[0].BitDepth, 24)
	eq(t, "dts atmos", a[0].IsAtmos(), false)
	eq(t, "core profile", a[1].Profile, "DTS")
	eq(t, "und", a[1].Language, "")
	eq(t, "default layout", a[1].ChannelLayout, "5.1")
	eq(t, "bitrate", a[1].Bitrate, 1509000)

	s := i.SubtitleStreams()
	eq(t, "subs", len(s), 3)
	eq(t, "vobsub image", s[0].IsImageSub, true)
	eq(t, "vobsub HI", s[0].HearingImpaired, true)
	eq(t, "vobsub lang es", s[0].Language, "spa")
	eq(t, "dvb image", s[1].IsImageSub, true)
	eq(t, "dvb lang nld", s[1].Language, "dut")
	eq(t, "ass text", s[2].IsImageSub, false)
	eq(t, "ass lang pt-BR", s[2].Language, "por")
	eq(t, "ass forced from title", s[2].Forced, true)
	eq(t, "ass default", s[2].Default, true)

	eq(t, "data type", i.Streams[6].Type, TypeData)
}

func TestGoldenHDR10PlusAndDVCodecTag(t *testing.T) {
	i := loadFixture(t, "hand_hdr10plus_dvtag.json")
	eq(t, "container m4v by ext", i.Container, "m4v")
	eq(t, "bitrate derived", i.Bitrate, 1000000)
	v := i.Video()
	eq(t, "hdr10+", v.HDR, HDR10Plus)
	near(t, "fps falls back to r_frame_rate", v.FrameRate, 24)
	eq(t, "dv by codec tag", i.Streams[1].HDR, DOVI)
	eq(t, "dv profile unknown", i.Streams[1].DoviProfile, 0)
	a := i.AudioStreams()[0]
	eq(t, "lang by name", a.Language, "jpn")
	eq(t, "he-aac", a.Profile, "HE-AACv2")
	eq(t, "zho→chi", i.SubtitleStreams()[0].Language, "chi")
	eq(t, "mov_text text", i.SubtitleStreams()[0].IsTextSub(), true)
}

func TestGoldenOggStreamTags(t *testing.T) {
	i := loadFixture(t, "hand_ogg_tags.json")
	eq(t, "container", i.Container, "ogg")
	eq(t, "duration from stream", i.DurationMs, 200000)
	eq(t, "title", i.Tags["title"], "Ogg Song")
	eq(t, "track alias", i.Tags["track"], "7")
	eq(t, "tracknumber kept", i.Tags["tracknumber"], "7")
	eq(t, "date", i.Tags["date"], "1999")
	if _, ok := i.Tags["encoder"]; ok {
		t.Error("technical stream tags must not leak into format tags")
	}
}

func TestParseOutputErrors(t *testing.T) {
	for _, name := range []string{"hand_invalid.json", "hand_nostreams.json"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseOutput(data, ""); !errors.Is(err, ErrNotMedia) {
			t.Errorf("%s: err = %v, want ErrNotMedia", name, err)
		}
	}
	if _, err := ParseOutput([]byte("{not json"), ""); err == nil || errors.Is(err, ErrNotMedia) {
		t.Errorf("bad json: err = %v, want a non-ErrNotMedia error", err)
	}
	if _, err := ParseOutput([]byte(`{}`), ""); !errors.Is(err, ErrNotMedia) {
		t.Errorf("empty object: err = %v, want ErrNotMedia", err)
	}
}

func TestParseRational(t *testing.T) {
	for in, want := range map[string]float64{
		"24000/1001": 23.976023976,
		"30000/1001": 29.97002997,
		"25/1":       25,
		"25":         25,
		"0/0":        0,
		"1/0":        0,
		"":           0,
		"abc":        0,
		"x/2":        0,
		"-25/1":      0,
		"16:9":       16.0 / 9,
		" 50/2 ":     25,
		"NaN/1":      0,
		"Inf":        0,
	} {
		if got := ParseRational(in); math.Abs(got-want) > 1e-6 {
			t.Errorf("ParseRational(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNormaliseLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"fre": "fre", "fra": "fre", "fr": "fre", "FR": "fre", "fr-CA": "fre", "fr_FR": "fre",
		"eng": "eng", "en": "eng", "en-US": "eng", "English": "eng",
		"deu": "ger", "ger": "ger", "de": "ger",
		"jpn": "jpn", "ja": "jpn", "jap": "jpn",
		"zho": "chi", "zh": "chi", "chi": "chi",
		"nld": "dut", "nl": "dut", "ces": "cze", "cs": "cze", "ell": "gre", "el": "gre",
		"ron": "rum", "slk": "slo", "fas": "per", "isl": "ice", "cym": "wel", "sqi": "alb",
		"hye": "arm", "eus": "baq", "mya": "bur", "kat": "geo", "mkd": "mac", "mri": "mao",
		"msa": "may", "bod": "tib", "iw": "heb", "he": "heb",
		"spa": "spa", "es": "spa", "pt-BR": "por", "kor": "kor", "tlh": "tlh",
		"und": "", "UND": "", "unk": "", "zxx": "", "": "", " ": "", "xx": "", "e": "",
		"english2": "", "12": "", "abcd": "",
	} {
		if got := NormaliseLanguage(in); got != want {
			t.Errorf("NormaliseLanguage(%q) = %q, want %q", in, got, want)
		}
	}
	// Every 2-letter mapping must produce a 3-letter lowercase B code that is
	// itself stable under normalisation.
	for two, b := range iso6391 {
		if len(b) != 3 || NormaliseLanguage(b) != b {
			t.Errorf("iso6391[%q] = %q is not a stable B code", two, b)
		}
	}
}

func TestNormaliseContainer(t *testing.T) {
	const mp4 = "mov,mp4,m4a,3gp,3g2,mj2"
	cases := []struct{ format, path, brand, want string }{
		{"matroska,webm", "a.mkv", "", "mkv"},
		{"matroska,webm", "a.MKV", "", "mkv"},
		{"matroska,webm", "a.webm", "", "webm"},
		{"matroska,webm", "", "", "mkv"},
		{mp4, "a.mp4", "isom", "mp4"},
		{mp4, "a.m4a", "", "m4a"},
		{mp4, "a.m4b", "", "m4a"},
		{mp4, "a.mov", "", "mov"},
		{mp4, "a.3gp", "", "3gp"},
		{mp4, "a.m4v", "", "m4v"},
		{mp4, "noext", "qt  ", "mov"},
		{mp4, "noext", "M4A ", "m4a"},
		{mp4, "noext", "", "mp4"},
		{"mpegts", "a.ts", "", "ts"},
		{"mpegts", "a.m2ts", "", "ts"},
		{"mpeg", "a.vob", "", "vob"},
		{"mpeg", "a.mpg", "", "mpeg"},
		{"avi", "a.avi", "", "avi"},
		{"asf", "a.wmv", "", "wmv"},
		{"asf", "a.asf", "", "asf"},
		{"ogg", "a.ogg", "", "ogg"},
		{"ogg", "a.opus", "", "opus"},
		{"flac", "a.flac", "", "flac"},
		{"mp3", "a.mp3", "", "mp3"},
		{"wav", "a.wav", "", "wav"},
		{"webvtt", "a.vtt", "", "vtt"},
		{"png_pipe", "a.png", "", "png"},
		{"jpeg_pipe", "a.jpg", "", "jpg"},
		{"flv", "a.flv", "", "flv"},
		{"", "a.xyz", "", "xyz"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "http://x/a.mov?token=1", "", "mov"},
	}
	for _, c := range cases {
		if got := NormaliseContainer(c.format, c.path, c.brand); got != c.want {
			t.Errorf("NormaliseContainer(%q, %q, %q) = %q, want %q", c.format, c.path, c.brand, got, c.want)
		}
	}
}

func TestBitDepthFromPixFmt(t *testing.T) {
	for in, want := range map[string]int{
		"yuv420p": 8, "nv12": 8, "rgb24": 8, "yuvj420p": 8, "yuv420p10le": 10, "yuv420p10be": 10,
		"p010le": 10, "p016le": 16, "yuv444p12le": 12, "gray10le": 10, "gbrp10le": 10, "": 0,
	} {
		if got := bitDepthFromPixFmt(in); got != want {
			t.Errorf("bitDepthFromPixFmt(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseClockMs(t *testing.T) {
	for in, want := range map[string]int64{
		"00:00:01.023000000": 1023, "02:10:03.042000000": 7803042, "bad": 0, "": 0, "1:2": 0,
	} {
		if got := parseClockMs(in); got != want {
			t.Errorf("parseClockMs(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	for in, want := range map[string]string{
		"ffprobe version 6.1.1-3ubuntu5 Copyright (c) 2007-2023 the FFmpeg developers\nbuilt with gcc": "6.1.1",
		"ffprobe version 7.0 Copyright (c) 2007-2024":                                                  "7.0",
		"ffprobe version n6.1.1-1-gabc Copyright":                                                      "n6.1.1-1-gabc",
		"ffprobe version N-113000-g1234567 Copyright":                                                  "N-113000-g1234567",
		"ffprobe version 5.1.4-0+deb12u1 Copyright (c) 2007-2023 the FFmpeg":                           "5.1.4",
		"garbage": "",
	} {
		if got := parseVersion(in); got != want {
			t.Errorf("parseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInputArg(t *testing.T) {
	for in, want := range map[string]string{
		"/media/a.mkv":      "file:/media/a.mkv",
		"-i.mkv":            "file:-i.mkv",
		"weird: name.mkv":   "file:weird: name.mkv",
		"http://host/a.mkv": "http://host/a.mkv",
	} {
		got, err := inputArg(in)
		if err != nil || got != want {
			t.Errorf("inputArg(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "-x://y", "a\x00b"} {
		if _, err := inputArg(bad); err == nil {
			t.Errorf("inputArg(%q) succeeded, want error", bad)
		}
	}
}

func TestHelpersNilSafe(t *testing.T) {
	var i *Info
	if i.Video() != nil || i.AudioStreams() != nil || i.SubtitleStreams() != nil || i.AttachedPic() != nil {
		t.Error("nil Info helpers must return nil")
	}
}
