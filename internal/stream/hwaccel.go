package stream

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
)

// hwEncoders are the hardware H.264 encoders we know how to drive, in
// preference order.
var hwEncoders = []string{"h264_vaapi", "h264_qsv", "h264_nvenc", "h264_videotoolbox"}

const defaultVAAPIDevice = "/dev/dri/renderD128"

func hwModeOfEncoder(enc string) string {
	switch enc {
	case "h264_vaapi":
		return HWVAAPI
	case "h264_qsv":
		return HWQSV
	case "h264_nvenc":
		return HWNVENC
	case "h264_videotoolbox":
		return HWVideoToolbox
	}
	return HWNone
}

// hwTestArgs is a one-frame test encode with the given encoder.
func hwTestArgs(enc string) []string {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
	switch enc {
	case "h264_vaapi":
		args = append(args, "-init_hw_device", "vaapi=va:"+defaultVAAPIDevice, "-filter_hw_device", "va")
	case "h264_qsv":
		args = append(args, "-init_hw_device", "qsv=qs", "-filter_hw_device", "qs")
	}
	args = append(args, "-f", "lavfi", "-i", "color=c=black:s=256x256:r=25", "-frames:v", "1")
	switch enc {
	case "h264_vaapi":
		args = append(args, "-vf", "format=nv12,hwupload")
	case "h264_qsv":
		args = append(args, "-vf", "format=nv12,hwupload=extra_hw_frames=16")
	default:
		args = append(args, "-pix_fmt", "yuv420p")
	}
	return append(args, "-c:v", enc, "-f", "null", "-")
}

// DetectHWAccel returns the hardware H.264 encoders that actually
// initialise (h264_vaapi, h264_qsv, h264_nvenc, h264_videotoolbox): each
// encoder listed by `ffmpeg -encoders` is tried with a one-frame encode
// under a short timeout. An empty result means software only.
func DetectHWAccel(ctx context.Context, ffmpeg string) []string {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	out, err := exec.CommandContext(lctx, ffmpeg, "-hide_banner", "-encoders").Output()
	cancel()
	if err != nil {
		return nil
	}
	listed := map[string]bool{}
	for _, line := range bytes.Split(out, []byte("\n")) {
		if f := bytes.Fields(line); len(f) >= 2 {
			listed[string(f[1])] = true
		}
	}
	var ok []string
	for _, enc := range hwEncoders {
		if !listed[enc] {
			continue
		}
		if enc == "h264_vaapi" {
			if _, err := os.Stat(defaultVAAPIDevice); err != nil {
				continue
			}
		}
		tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := exec.CommandContext(tctx, ffmpeg, hwTestArgs(enc)...).Run()
		cancel()
		if err == nil {
			ok = append(ok, enc)
		}
	}
	return ok
}
