package stream

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// ExtractSubtitle converts text subtitle stream streamIndex of mediaPath
// (an internal stream, or an external .srt/.ass/.vtt file with index 0) to
// a WebVTT file at outPath. mediaPath may be an http(s) URL. The file is written atomically (temp + rename).
func ExtractSubtitle(ctx context.Context, ffmpeg, mediaPath string, streamIndex int, outPath string) error {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if streamIndex < 0 {
		return errors.New("stream: invalid subtitle stream index")
	}
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(outPath)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpName) }()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	stderr := &tailBuffer{max: 4 << 10}
	cmd := exec.CommandContext(ctx, ffmpeg,
		append(append([]string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}, inputOptions(mediaPath)...),
			"-i", inputArg(mediaPath),
			"-map", "0:"+strconv.Itoa(streamIndex),
			"-c:s", "webvtt", "-f", "webvtt", inputArg(tmpName))...)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("stream: extract subtitle: %w", ctx.Err())
		}
		return fmt.Errorf("stream: extract subtitle: %w: %s", err, stderr.lastLines(5))
	}
	return os.Rename(tmpName, outPath)
}
