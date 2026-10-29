// Package stream decides how a media file is delivered to a client (direct
// play, remux or transcode) and runs the FFmpeg sessions that produce
// on-demand HLS (fMP4) for the remux and transcode cases.
package stream

import (
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
)

// Profile mirrors ClientProfile in docs/API.md: what the browser can play.
type Profile struct {
	Containers  []string `json:"containers"`
	VideoCodecs []string `json:"videoCodecs"`
	AudioCodecs []string `json:"audioCodecs"`
	HDR         bool     `json:"hdr"`
	MaxHeight   int      `json:"maxHeight,omitempty"`
	MaxBitrate  int64    `json:"maxBitrate,omitempty"`
	HLS         string   `json:"hls"` // "native" | "mse"
}

// Method is the delivery method chosen by Decide.
type Method string

// Delivery methods.
const (
	MethodDirect    Method = "direct"
	MethodRemux     Method = "remux"
	MethodTranscode Method = "transcode"
)

// Reason codes explaining why a file is not played directly (stable, part
// of the HTTP contract).
const (
	ReasonContainer    = "container"
	ReasonVideoCodec   = "videoCodec"
	ReasonAudioCodec   = "audioCodec"
	ReasonAudioTrack   = "audioTrack"
	ReasonBitrate      = "bitrate"
	ReasonResolution   = "resolution"
	ReasonHDR          = "hdr"
	ReasonSubtitleBurn = "subtitleBurn"
)

// Request describes one playback request.
type Request struct {
	Info          *probe.Info // probed file (Container normalised: mkv, mp4, webm, ts, avi, ...)
	Path          string      // media file path
	AudioIndex    int         // ffprobe stream index, -1 = pick default
	SubtitleIndex int         // ffprobe stream index, -1 = none
	Profile       Profile
	StartMs       int64
	Language      string // preferred audio language (ISO 639-2/B, e.g. "fre"), optional
	// BurnSubtitle forces burn-in of the selected subtitle even when it is a
	// text subtitle (which is normally delivered as a WebVTT side-car).
	// Image subtitles are always burnt.
	BurnSubtitle bool
	// Source locates the file for the keyframe index (remux); optional.
	Source Source
}

// Decision is the outcome of Decide.
type Decision struct {
	Method        Method
	Reasons       []string // why not direct; stable codes (Reason*)
	AudioIndex    int      // selected audio stream index, -1 = none
	SubtitleIndex int      // selected subtitle stream index, -1 = none
	BurnSubtitle  bool     // subtitle burnt into the video → video transcoded
	VideoCopy     bool
	AudioCopy     bool
	ToneMap       bool  // HDR source, SDR client
	TargetHeight  int   // 0 = source height
	VideoBitrate  int64 // target for transcode (bits/s)
}

// Encoding defaults.
const (
	audioTranscodeBitrate = 192_000
	minVideoBitrate       = 200_000
)

// Decide chooses the delivery method. It is pure (no I/O).
//
// Rules:
//
//   - direct: the container is in Profile.Containers AND the video codec is
//     supported (hevc/av1/vp9 only when listed; H.264 only 8-bit; an HDR
//     source only when Profile.HDR, otherwise it must be tone-mapped, which
//     means transcoding) AND the selected audio codec is supported AND the
//     selected audio stream is the first audio stream (browsers cannot switch
//     tracks in direct play) AND nothing is burnt in AND the file bitrate is
//     ≤ MaxBitrate (if set) AND the height ≤ MaxHeight (if set).
//     Audio-only files: container + audio rules only.
//   - remux (video copy to HLS fMP4): the video itself is acceptable (codec,
//     HDR, size, bitrate, no burn-in) and can be carried in fMP4 (h264, hevc,
//     av1; vp9 only with MSE players), but the container, the audio track
//     selection or the audio codec prevents direct play. Unsupported audio is
//     transcoded to AAC stereo. Audio-only files whose codec can be copied
//     into fMP4 are remuxed the same way.
//   - transcode: anything else. Video becomes H.264 (scaled to TargetHeight,
//     tone-mapped when ToneMap, subtitles burnt when BurnSubtitle); audio is
//     copied when the client supports it in fMP4, else AAC stereo 192 kb/s.
func Decide(r Request) Decision {
	d := Decision{AudioIndex: -1, SubtitleIndex: -1}
	info := r.Info
	p := r.Profile
	if info == nil {
		d.Method = MethodTranscode
		return d
	}

	audio := selectAudio(info, r.AudioIndex, r.Language)
	if audio != nil {
		d.AudioIndex = audio.Index
	}
	if r.SubtitleIndex >= 0 {
		if s := streamByIndex(info, r.SubtitleIndex); s != nil && s.Type == probe.TypeSubtitle {
			d.SubtitleIndex = s.Index
			d.BurnSubtitle = s.IsImageSub || r.BurnSubtitle
		}
	}

	var reasons []string
	add := func(code string) {
		for _, c := range reasons {
			if c == code {
				return
			}
		}
		reasons = append(reasons, code)
	}

	video := info.Video()
	if !containerSupported(info.Container, p.Containers) {
		add(ReasonContainer)
	}

	// Video conditions: any of them forbids copying the video.
	videoBlocked := false
	if video != nil {
		if !videoCodecSupported(video, p) {
			add(ReasonVideoCodec)
			videoBlocked = true
		}
		if isHDR(video) && !hdrSupported(video, p) {
			add(ReasonHDR)
			d.ToneMap = isToneMappable(video)
			videoBlocked = true
		}
		if p.MaxHeight > 0 && video.Height > p.MaxHeight {
			add(ReasonResolution)
			videoBlocked = true
		}
		if p.MaxBitrate > 0 && info.Bitrate > p.MaxBitrate {
			add(ReasonBitrate)
			videoBlocked = true
		}
		if d.BurnSubtitle {
			add(ReasonSubtitleBurn)
			videoBlocked = true
		}
	} else if p.MaxBitrate > 0 && info.Bitrate > p.MaxBitrate {
		add(ReasonBitrate)
	}

	audioDirect := true
	if audio != nil {
		if !codecIn(audio.Codec, p.AudioCodecs) {
			add(ReasonAudioCodec)
			audioDirect = false
		}
		if first := firstAudio(info); first != nil && first.Index != audio.Index {
			add(ReasonAudioTrack)
		}
	}
	d.AudioCopy = audio != nil && audioDirect && audioCopyable(audio.Codec)

	if len(reasons) == 0 {
		d.Method = MethodDirect
		d.VideoCopy = video != nil
		d.AudioCopy = audio != nil
		return d
	}
	if video == nil {
		// Audio-only: copy the audio into fMP4 when possible.
		if d.AudioCopy && !hasReason(reasons, ReasonBitrate) {
			d.Method = MethodRemux
		} else {
			d.Method = MethodTranscode
			d.AudioCopy = false
		}
		d.Reasons = reasons
		return d
	}
	if !videoBlocked && fmp4VideoCopyable(video.Codec, p.HLS) {
		d.Method = MethodRemux
		d.VideoCopy = true
		d.Reasons = reasons
		return d
	}
	if !videoBlocked {
		// Playable by the client but not carriable in fMP4 HLS (vp8, vp9
		// for native HLS...): the video codec is what forces transcoding.
		add(ReasonVideoCodec)
	}
	d.Method = MethodTranscode
	d.Reasons = reasons
	d.TargetHeight, d.VideoBitrate = transcodeTarget(video, audio, d.AudioCopy, p)
	return d
}

// transcodeTarget picks the output height (0 = source) and the target video
// bitrate from MaxHeight / MaxBitrate and the default ladder.
func transcodeTarget(v, a *probe.Stream, audioCopy bool, p Profile) (int, int64) {
	src := v.Height
	target := src
	if target <= 0 {
		target = 1080
	}
	if p.MaxHeight > 0 && target > p.MaxHeight {
		target = p.MaxHeight
	}
	var audioBr int64
	if a != nil {
		audioBr = audioTranscodeBitrate
		if audioCopy && a.Bitrate > 0 {
			audioBr = a.Bitrate
		}
	}
	var budget int64
	if p.MaxBitrate > 0 {
		budget = p.MaxBitrate - audioBr
		if budget < minVideoBitrate {
			budget = minVideoBitrate
		}
		if hb := heightForBitrate(budget); hb < target {
			target = hb
		}
	}
	br := ladderBitrate(target)
	if budget > 0 && br > budget {
		br = budget
	}
	if src > 0 && target >= src {
		target = 0
	}
	return target, br
}

// ladderBitrate is the default target bitrate for an output height:
// 2160→20 Mb/s, 1080→8, 720→4, 480 and below→1.5.
func ladderBitrate(h int) int64 {
	switch {
	case h > 1080:
		return 20_000_000
	case h > 720:
		return 8_000_000
	case h > 480:
		return 4_000_000
	default:
		return 1_500_000
	}
}

// heightForBitrate is the largest sensible output height for a video budget.
func heightForBitrate(b int64) int {
	switch {
	case b >= 10_000_000:
		return 2160
	case b >= 4_000_000:
		return 1080
	case b >= 2_000_000:
		return 720
	case b >= 1_000_000:
		return 480
	default:
		return 360
	}
}

// selectAudio returns the requested audio stream, or the default choice:
// a stream in the preferred language (its default one first), else the
// stream flagged default, else the first audio stream.
func selectAudio(info *probe.Info, index int, lang string) *probe.Stream {
	if index >= 0 {
		if s := streamByIndex(info, index); s != nil && s.Type == probe.TypeAudio {
			return s
		}
	}
	var first, def, langFirst, langDef *probe.Stream
	for k := range info.Streams {
		s := &info.Streams[k]
		if s.Type != probe.TypeAudio {
			continue
		}
		if first == nil {
			first = s
		}
		if s.Default && def == nil {
			def = s
		}
		if lang != "" && strings.EqualFold(s.Language, lang) {
			if langFirst == nil {
				langFirst = s
			}
			if s.Default && langDef == nil {
				langDef = s
			}
		}
	}
	for _, s := range []*probe.Stream{langDef, langFirst, def, first} {
		if s != nil {
			return s
		}
	}
	return nil
}

func streamByIndex(info *probe.Info, index int) *probe.Stream {
	for k := range info.Streams {
		if info.Streams[k].Index == index {
			return &info.Streams[k]
		}
	}
	return nil
}

func firstAudio(info *probe.Info) *probe.Stream {
	for k := range info.Streams {
		if info.Streams[k].Type == probe.TypeAudio {
			return &info.Streams[k]
		}
	}
	return nil
}

// relativeSubtitleIndex returns the position of stream index among the
// subtitle streams (the "si" of the subtitles filter), or -1.
func relativeSubtitleIndex(info *probe.Info, index int) int {
	n := 0
	for _, s := range info.Streams {
		if s.Type != probe.TypeSubtitle {
			continue
		}
		if s.Index == index {
			return n
		}
		n++
	}
	return -1
}

func hasReason(rs []string, code string) bool {
	for _, r := range rs {
		if r == code {
			return true
		}
	}
	return false
}

// containerSupported matches a normalised container against the profile.
// "m4a" is an MP4 and "mov" is accepted when "mov" is listed.
func containerSupported(c string, list []string) bool {
	c = strings.ToLower(c)
	if c == "" {
		return false
	}
	for _, l := range list {
		l = strings.ToLower(l)
		if l == c || (c == "m4a" && l == "mp4") || (c == "oga" && l == "ogg") {
			return true
		}
	}
	return false
}

// normCodec maps ffprobe codec names onto profile codec names.
func normCodec(c string) string {
	c = strings.ToLower(c)
	switch {
	case strings.HasPrefix(c, "pcm_"):
		return "pcm"
	case c == "h265":
		return "hevc"
	case c == "avc":
		return "h264"
	}
	return c
}

func codecIn(c string, list []string) bool {
	c = normCodec(c)
	if c == "" {
		return false
	}
	for _, l := range list {
		if normCodec(l) == c {
			return true
		}
	}
	return false
}

// videoCodecSupported reports whether the client decodes the video stream.
// Browsers do not decode 10-bit H.264 (Hi10P), whatever they announce.
func videoCodecSupported(v *probe.Stream, p Profile) bool {
	if !codecIn(v.Codec, p.VideoCodecs) {
		return false
	}
	if normCodec(v.Codec) == "h264" && v.BitDepth > 8 {
		return false
	}
	return true
}

// isHDR reports whether the stream needs an HDR display (PQ/HLG transfer, or
// Dolby Vision without an SDR-compatible base layer).
func isHDR(v *probe.Stream) bool {
	if v.HDR == probe.DOVI && v.DoviCompatID == 2 {
		return false // SDR base layer
	}
	if v.HDR != "" {
		return true
	}
	switch v.ColorTransfer {
	case "smpte2084", "arib-std-b67":
		return true
	}
	return false
}

// hdrSupported: the client displays HDR, and for Dolby Vision without a
// backward-compatible base layer (profile 5) it must list "dvh1".
func hdrSupported(v *probe.Stream, p Profile) bool {
	if !p.HDR {
		return false
	}
	if v.HDR == probe.DOVI && v.DoviCompatID != 1 && v.DoviCompatID != 4 {
		return codecIn("dvh1", p.VideoCodecs)
	}
	return true
}

// isToneMappable: PQ/HLG sources (and DV with an HDR10/HLG base layer) are
// tone-mapped. DV profile 5 (IPT-PQ-C2) cannot be tone-mapped correctly in
// software; it is transcoded anyway (colours will be off).
func isToneMappable(v *probe.Stream) bool { return isHDR(v) }

// fmp4VideoCopyable: codecs browsers accept in fMP4 HLS when copied.
func fmp4VideoCopyable(c, hls string) bool {
	switch normCodec(c) {
	case "h264", "hevc", "av1":
		return true
	case "vp9":
		return hls != "native"
	}
	return false
}

// audioCopyable: audio codecs the fMP4 muxer carries and browsers decode
// from fMP4 (only used when the client also lists the codec).
func audioCopyable(c string) bool {
	switch normCodec(c) {
	case "aac", "mp3", "opus", "flac", "ac3", "eac3", "alac":
		return true
	}
	return false
}
