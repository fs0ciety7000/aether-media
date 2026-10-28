package stream

import (
	"fmt"
	"math"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
)

// videoCodecsString returns the RFC 6381 codec string of the output video.
func videoCodecsString(info *probe.Info, d Decision) string {
	v := info.Video()
	if v == nil {
		return ""
	}
	if !d.VideoCopy {
		_, idc := h264Level(outputHeight(info, d))
		return fmt.Sprintf("avc1.6400%02x", idc)
	}
	switch normCodec(v.Codec) {
	case "h264":
		return avc1String(v)
	case "hevc":
		return hvc1String(v)
	case "av1":
		return av1String(v)
	case "vp9":
		return vp9String(v)
	}
	return ""
}

func avc1String(v *probe.Stream) string {
	prof := strings.ToLower(v.Profile)
	var pc string
	switch {
	case strings.Contains(prof, "constrained baseline"):
		pc = "42e0"
	case strings.Contains(prof, "baseline"):
		pc = "4200"
	case strings.Contains(prof, "main"):
		pc = "4d40"
	case strings.Contains(prof, "high 10"):
		pc = "6e00"
	case strings.Contains(prof, "high 4:2:2"):
		pc = "7a00"
	case strings.Contains(prof, "high 4:4:4"):
		pc = "f400"
	default:
		pc = "6400"
	}
	level := v.Level
	if level <= 0 {
		_, level = h264Level(v.Height)
	}
	return fmt.Sprintf("avc1.%s%02x", pc, level)
}

// hvc1String: hvc1.<profile>.<compat>.L<level*30>.B0 (main tier assumed).
func hvc1String(v *probe.Stream) string {
	prof, compat := 1, 6
	p := strings.ToLower(v.Profile)
	if strings.Contains(p, "main 10") || v.BitDepth > 8 {
		prof, compat = 2, 4
	}
	level := v.Level
	if level <= 0 {
		switch {
		case v.Height > 1080:
			level = 153
		case v.Height > 720:
			level = 123
		default:
			level = 93
		}
	}
	return fmt.Sprintf("hvc1.%d.%d.L%d.B0", prof, compat, level)
}

// av1String: av01.<profile>.<level><tier>.<bitdepth>.
func av1String(v *probe.Stream) string {
	prof := 0
	p := strings.ToLower(v.Profile)
	switch {
	case strings.Contains(p, "high"):
		prof = 1
	case strings.Contains(p, "professional"):
		prof = 2
	}
	level := v.Level
	if level <= 0 || level > 31 {
		switch {
		case v.Height > 1080:
			level = 12 // 5.0
		case v.Height > 720:
			level = 8 // 4.0
		default:
			level = 5 // 3.1
		}
	}
	depth := 8
	if v.BitDepth > 8 {
		depth = v.BitDepth
	}
	return fmt.Sprintf("av01.%d.%02dM.%02d", prof, level, depth)
}

// vp9String: vp09.<profile>.<level>.<bitdepth>.
func vp9String(v *probe.Stream) string {
	prof := 0
	depth := 8
	if v.BitDepth > 8 {
		prof, depth = 2, v.BitDepth
	}
	level := 40
	switch {
	case v.Height > 1080:
		level = 51
	case v.Height <= 720:
		level = 31
	}
	return fmt.Sprintf("vp09.%02d.%02d.%02d", prof, level, depth)
}

// audioCodecsString returns the RFC 6381 codec string of the output audio.
func audioCodecsString(info *probe.Info, d Decision) string {
	if d.AudioIndex < 0 {
		return ""
	}
	if !d.AudioCopy {
		return "mp4a.40.2"
	}
	a := streamByIndex(info, d.AudioIndex)
	if a == nil {
		return ""
	}
	switch normCodec(a.Codec) {
	case "aac":
		p := strings.ToLower(a.Profile)
		switch {
		case strings.Contains(p, "he-aacv2"):
			return "mp4a.40.29"
		case strings.Contains(p, "he-aac"):
			return "mp4a.40.5"
		}
		return "mp4a.40.2"
	case "mp3":
		return "mp4a.40.34"
	case "opus":
		return "Opus"
	case "flac":
		return "fLaC"
	case "ac3":
		return "ac-3"
	case "eac3":
		return "ec-3"
	case "alac":
		return "alac"
	}
	return ""
}

// outputSize returns the output video resolution (even dimensions).
func outputSize(info *probe.Info, d Decision) (int, int) {
	v := info.Video()
	if v == nil || v.Width <= 0 || v.Height <= 0 {
		return 0, 0
	}
	if d.VideoCopy {
		return v.Width, v.Height
	}
	h := outputHeight(info, d)
	// Same rounding as ffmpeg scale=-2: nearest even width.
	w := int(math.Round(float64(v.Width)*float64(h)/float64(v.Height)/2)) * 2
	return w, h
}

// bandwidth estimates the variant peak bitrate for master.m3u8.
func bandwidth(info *probe.Info, d Decision) int64 {
	var audio int64
	if d.AudioIndex >= 0 {
		audio = audioTranscodeBitrate
		if d.AudioCopy {
			if a := streamByIndex(info, d.AudioIndex); a != nil && a.Bitrate > 0 {
				audio = a.Bitrate
			}
		}
	}
	if info.Video() == nil {
		return audio + 16_000
	}
	var video int64
	if d.VideoCopy {
		v := info.Video()
		video = v.Bitrate
		if video <= 0 && info.Bitrate > 0 {
			video = info.Bitrate - audio
		}
		if video <= 0 {
			video = 10_000_000
		}
		video = video * 3 / 2 // peak vs average
	} else {
		video = d.VideoBitrate
		if video <= 0 {
			video = ladderBitrate(outputHeight(info, d))
		}
	}
	return video + audio
}
