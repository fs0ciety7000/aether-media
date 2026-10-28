package stream

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
)

// Hardware acceleration modes (Config.HWAccel).
const (
	HWNone         = "none"
	HWVAAPI        = "vaapi"
	HWQSV          = "qsv"
	HWNVENC        = "nvenc"
	HWVideoToolbox = "videotoolbox"
	HWAuto         = "auto"
)

// toneMapper is the software HDR→SDR strategy available in the FFmpeg build.
type toneMapper int

const (
	toneMapZscale     toneMapper = iota // zscale + tonemap=hable (accurate)
	toneMapColorspace                   // no zscale: primaries conversion only (approximate)
)

// File names inside a run directory.
const (
	initName       = "init.mp4"
	ffPlaylistName = "ff.m3u8"
	chunkPrefix    = "k" // keyframe chunks (video copy): k<i>.m4s
)

// jobParams is everything needed to build one FFmpeg command line.
type jobParams struct {
	Input       string
	Info        *probe.Info
	Decision    Decision
	HW          string // resolved: none|vaapi|qsv|nvenc|videotoolbox
	VAAPIDevice string
	ToneMapper  toneMapper
	SegmentSec  int
	Dir         string // run directory (absolute)
	Plan        *plan
	StartSeg    int // first playlist segment produced by this run
}

// segmentFile is the file name FFmpeg gives to output number n.
func segmentFile(keyframed bool, n int) string {
	if keyframed {
		return chunkPrefix + strconv.Itoa(n) + ".m4s"
	}
	return strconv.Itoa(n) + ".m4s"
}

func fmtSec(s float64) string { return strconv.FormatFloat(s, 'f', 3, 64) }

// buildArgs builds the FFmpeg argument list (no shell involved).
//
// Timestamps: the input is seeked with -ss (fast, before -i) and the output
// keeps the source timeline (-copyts -start_at_zero -avoid_negative_ts
// disabled): every run, whatever its start point, emits fragments whose
// tfdt is the absolute position in the file, so hls.js / Safari see
// monotonic, continuous timestamps across seek restarts.
//
// Segmentation: transcodes force keyframes on the SegmentSec grid and let
// the HLS muxer cut every SegmentSec (start_number = playlist index). Video
// copies cannot move keyframes, and the HLS muxer cuts relative to its own
// start point, so a restart would shift every later boundary; instead the
// muxer cuts at *every* source keyframe (tiny hls_time, start_number =
// keyframe index) and a playlist segment is served as the concatenation of
// its keyframe chunks — the layout is then independent of the restart point.
func buildArgs(p jobParams) []string {
	d := p.Decision
	info := p.Info
	video := info.Video()
	keyframed := p.Plan.keyframed()

	var seek float64
	startNumber := p.StartSeg
	if keyframed {
		startNumber = p.Plan.chunkStart[p.StartSeg]
		if p.StartSeg > 0 {
			seek = p.Plan.kf[startNumber] + seekPastKeyframe
		}
	} else if p.StartSeg > 0 {
		seek = p.Plan.starts[p.StartSeg]
	}

	transcodeVideo := video != nil && !d.VideoCopy
	hw := p.HW
	if !transcodeVideo {
		hw = HWNone
	}
	vs := videoSetup(p, hw)

	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}
	args = append(args, vs.input...)
	args = append(args, inputOptions(p.Input)...)
	if seek > 0 {
		args = append(args, "-ss", fmtSec(seek))
	}
	args = append(args, "-i", inputArg(p.Input))

	if video != nil {
		if transcodeVideo {
			args = append(args, "-filter_complex", vs.graph, "-map", "[v]")
		} else {
			args = append(args, "-map", "0:"+strconv.Itoa(video.Index))
		}
	}
	if d.AudioIndex >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(d.AudioIndex))
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1")

	if video != nil {
		if transcodeVideo {
			args = append(args, vs.encode...)
		} else {
			args = append(args, "-c:v", "copy")
			if normCodec(video.Codec) == "hevc" {
				args = append(args, "-tag:v", "hvc1")
			}
		}
	}
	if d.AudioIndex >= 0 {
		if d.AudioCopy {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", strconv.Itoa(audioTranscodeBitrate))
		}
	}

	args = append(args,
		"-copyts", "-avoid_negative_ts", "disabled", "-start_at_zero",
		"-max_muxing_queue_size", "2048",
		"-f", "hls")
	if keyframed {
		args = append(args, "-hls_time", "0.000001")
	} else {
		args = append(args, "-hls_time", strconv.Itoa(p.SegmentSec))
	}
	segOpts := "movflags=+frag_discont"
	if video != nil {
		segOpts += "+skip_sidx"
	}
	initFile := initName
	if runtime.GOOS == "windows" {
		initFile = filepath.Join(p.Dir, initName)
	}
	args = append(args,
		"-hls_segment_type", "fmp4",
		"-hls_playlist_type", "vod",
		"-hls_list_size", "0",
		"-hls_flags", "independent_segments+temp_file",
		"-hls_segment_options", segOpts,
		"-start_number", strconv.Itoa(startNumber),
		"-hls_fmp4_init_filename", initFile,
		"-hls_segment_filename", filepath.Join(escapeSegmentPattern(p.Dir), segmentPattern(keyframed)),
		filepath.Join(p.Dir, ffPlaylistName))
	return args
}

func segmentPattern(keyframed bool) string {
	if keyframed {
		return chunkPrefix + "%d.m4s"
	}
	return "%d.m4s"
}

// videoArgs groups the video-transcode parts of a command line.
type videoArgs struct {
	input  []string // before -i (hwaccel)
	graph  string   // -filter_complex, output label [v]
	encode []string // encoder options
}

// hwDecodable: codecs whose decode we keep on the GPU (frames never leave
// it); other codecs are decoded in software and uploaded.
func hwDecodable(codec string) bool {
	switch normCodec(codec) {
	case "h264", "hevc", "vp9", "av1", "mpeg2video":
		return true
	}
	return false
}

// videoSetup builds decode/filter/encode arguments for a video transcode.
//
// Full hardware path (hw decode → hw scale → hw encode) when no software
// filter is needed. Tone mapping and subtitle burn-in run in software (hw
// tone mapping is out of scope): frames are decoded in software, filtered,
// then uploaded to the encoder's device when it needs it (VAAPI/QSV).
func videoSetup(p jobParams, hw string) videoArgs {
	d := p.Decision
	video := p.Info.Video()
	if video == nil {
		return videoArgs{}
	}
	h := d.TargetHeight &^ 1
	needSW := d.ToneMap || d.BurnSubtitle || (video.Interlaced && hw == HWQSV)
	fullHW := hw != HWNone && hw != HWVideoToolbox && !needSW && hwDecodable(video.Codec)

	var va videoArgs
	in := "[0:" + strconv.Itoa(video.Index) + "]"
	var chain []string

	if fullHW {
		switch hw {
		case HWVAAPI:
			va.input = []string{"-hwaccel", "vaapi", "-hwaccel_device", p.VAAPIDevice, "-hwaccel_output_format", "vaapi"}
			if video.Interlaced {
				chain = append(chain, "deinterlace_vaapi")
			}
			if h > 0 {
				chain = append(chain, "scale_vaapi=w=-2:h="+strconv.Itoa(h)+":format=nv12")
			} else {
				chain = append(chain, "scale_vaapi=format=nv12")
			}
		case HWQSV:
			va.input = []string{"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}
			if h > 0 {
				chain = append(chain, "scale_qsv=w=-1:h="+strconv.Itoa(h)+":format=nv12")
			} else {
				chain = append(chain, "scale_qsv=format=nv12")
			}
		case HWNVENC:
			va.input = []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}
			if video.Interlaced {
				chain = append(chain, "yadif_cuda")
			}
			if h > 0 {
				chain = append(chain, "scale_cuda=w=-2:h="+strconv.Itoa(h)+":format=yuv420p")
			} else {
				chain = append(chain, "scale_cuda=format=yuv420p")
			}
		}
		va.graph = in + strings.Join(chain, ",") + "[v]"
	} else {
		switch hw {
		case HWVAAPI:
			va.input = []string{"-init_hw_device", "vaapi=va:" + p.VAAPIDevice, "-filter_hw_device", "va"}
		case HWQSV:
			va.input = []string{"-init_hw_device", "qsv=qs", "-filter_hw_device", "qs"}
		case HWVideoToolbox:
			va.input = []string{"-hwaccel", "videotoolbox"}
		}
		va.graph = softwareGraph(p, in, h, hw)
	}
	va.encode = encoderArgs(p, hw, h)
	return va
}

// softwareGraph builds the CPU filter graph: deinterlace → tone map →
// subtitle burn-in → scale → pixel format (→ upload for VAAPI/QSV).
func softwareGraph(p jobParams, in string, h int, hw string) string {
	d := p.Decision
	video := p.Info.Video()
	var pre []string
	if video.Interlaced {
		pre = append(pre, "yadif=mode=send_frame:parity=auto:deint=all")
	}
	if d.ToneMap {
		pre = append(pre, toneMapFilters(p.ToneMapper)...)
	}

	var post []string
	burnText := false
	var sub *probe.Stream
	if d.BurnSubtitle && d.SubtitleIndex >= 0 {
		sub = streamByIndex(p.Info, d.SubtitleIndex)
		burnText = sub != nil && !sub.IsImageSub
	}
	if burnText {
		si := relativeSubtitleIndex(p.Info, d.SubtitleIndex)
		pre = append(pre, "subtitles=filename="+escapeFilterGraph(escapeFilterValue(p.Input))+":si="+strconv.Itoa(max(si, 0)))
	}
	if h > 0 {
		post = append(post, "scale=w=-2:h="+strconv.Itoa(h))
	} else if video.Width%2 != 0 || video.Height%2 != 0 {
		post = append(post, "scale=w=trunc(iw/2)*2:h=trunc(ih/2)*2")
	}
	switch hw {
	case HWVAAPI:
		post = append(post, "format=nv12", "hwupload")
	case HWQSV:
		post = append(post, "format=nv12", "hwupload=extra_hw_frames=64")
	default:
		post = append(post, "format=yuv420p")
	}

	if sub != nil && sub.IsImageSub {
		base := in
		if len(pre) > 0 {
			base = "[base]"
		}
		var b strings.Builder
		if len(pre) > 0 {
			b.WriteString(in + strings.Join(pre, ",") + "[base];")
		}
		// Image subtitles are scaled to the video size (PGS canvases may
		// differ) and overlaid before the output scaling.
		fmt.Fprintf(&b, "[0:%d]scale=w=%d:h=%d:force_original_aspect_ratio=decrease[sub];", sub.Index, max(video.Width, 2), max(video.Height, 2))
		fmt.Fprintf(&b, "%s[sub]overlay=eof_action=pass:x=(W-w)/2:y=H-h,%s[v]", base, strings.Join(post, ","))
		return b.String()
	}
	return in + strings.Join(append(pre, post...), ",") + "[v]"
}

// toneMapFilters converts PQ/HLG BT.2020 to SDR BT.709.
func toneMapFilters(tm toneMapper) []string {
	if tm == toneMapZscale {
		return []string{
			"zscale=t=linear:npl=100",
			"format=gbrpf32le",
			"zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0",
			"zscale=t=bt709:m=bt709:r=tv",
			"format=yuv420p",
		}
	}
	// Fallback without libzimg: convert primaries/matrix only; highlights
	// are clipped and the picture looks flat, but colours are not green.
	return []string{"colorspace=all=bt709:iall=bt2020:itrc=bt2020-10:fast=1", "format=yuv420p"}
}

// h264Level is the H.264 level we declare for an output height.
func h264Level(h int) (name string, idc int) {
	switch {
	case h <= 0 || h > 1080:
		return "5.1", 51
	case h > 720:
		return "4.1", 41
	case h > 480:
		return "3.1", 31
	default:
		return "3.0", 30
	}
}

// outputHeight is the transcoded height (TargetHeight, else the source).
func outputHeight(info *probe.Info, d Decision) int {
	if d.TargetHeight > 0 {
		return d.TargetHeight &^ 1
	}
	if v := info.Video(); v != nil {
		return v.Height &^ 1
	}
	return 0
}

// gopFrames is the keyframe interval in frames for one segment.
func gopFrames(v *probe.Stream, seg int) int {
	fps := v.FrameRate
	if fps <= 0 || fps > 240 {
		fps = 25
	}
	return int(math.Ceil(fps * float64(seg)))
}

// encoderArgs returns the H.264 encoder options, including keyframe
// placement on the segment grid.
func encoderArgs(p jobParams, hw string, h int) []string {
	d := p.Decision
	video := p.Info.Video()
	br := d.VideoBitrate
	if br <= 0 {
		br = ladderBitrate(outputHeight(p.Info, d))
	}
	brs := strconv.FormatInt(br, 10)
	bufs := strconv.FormatInt(2*br, 10)
	if h <= 0 {
		h = outputHeight(p.Info, d)
	}
	level, _ := h264Level(h)
	gop := strconv.Itoa(gopFrames(video, p.SegmentSec))
	start := 0.0
	if p.StartSeg > 0 {
		start = p.Plan.starts[p.StartSeg]
	}
	// Force a keyframe at every segment boundary of this run's timeline
	// (t is the absolute, copyts timestamp).
	forceKF := fmt.Sprintf("expr:gte(t,%s+n_forced*%d)", fmtSec(start), p.SegmentSec)

	var a []string
	switch hw {
	case HWVAAPI:
		a = []string{"-c:v", "h264_vaapi", "-rc_mode", "VBR", "-b:v", brs, "-maxrate", brs, "-bufsize", bufs,
			"-profile:v", "high", "-force_key_frames", forceKF}
	case HWQSV:
		a = []string{"-c:v", "h264_qsv", "-preset", "veryfast", "-look_ahead", "0", "-b:v", brs, "-maxrate", brs, "-bufsize", bufs,
			"-profile:v", "high", "-forced_idr", "1"}
	case HWNVENC:
		a = []string{"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr", "-b:v", brs, "-maxrate", brs, "-bufsize", bufs,
			"-profile:v", "high", "-forced-idr", "1", "-force_key_frames", forceKF}
	case HWVideoToolbox:
		a = []string{"-c:v", "h264_videotoolbox", "-allow_sw", "1", "-b:v", brs, "-maxrate", brs, "-bufsize", bufs,
			"-profile:v", "high", "-force_key_frames", forceKF}
	default:
		a = []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-maxrate", brs, "-bufsize", bufs,
			"-profile:v", "high", "-level:v", level, "-force_key_frames", forceKF, "-sc_threshold", "0"}
	}
	// Bound the GOP to one segment: encoders that ignore forced keyframes
	// (QSV) rely on it, and it stops scene-cut keyframes from pushing the
	// next forced one past a boundary.
	a = append(a, "-g", gop, "-keyint_min", gop)
	return a
}
