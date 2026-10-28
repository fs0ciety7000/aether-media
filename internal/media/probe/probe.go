// Package probe runs ffprobe on media files and normalises its JSON output
// into a stable, codec-agnostic description (container, streams, chapters,
// tags) that the rest of Aether stores and reasons about.
package probe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNotMedia is returned when ffprobe cannot recognise the input as media
// (invalid data) or when the file contains no usable stream.
var ErrNotMedia = errors.New("probe: not a media file")

const (
	defaultBinary  = "ffprobe"
	defaultTimeout = 60 * time.Second
	maxStderr      = 4 << 10
)

// Prober runs ffprobe. The zero value is usable and uses the defaults.
type Prober struct {
	FFprobe string        // binary path, default "ffprobe"
	Timeout time.Duration // per file, default 60s
}

// New returns a Prober using the given ffprobe binary ("" means "ffprobe" from PATH).
func New(ffprobe string) *Prober {
	if ffprobe == "" {
		ffprobe = defaultBinary
	}
	return &Prober{FFprobe: ffprobe, Timeout: defaultTimeout}
}

func (p *Prober) binary() string {
	if p == nil || p.FFprobe == "" {
		return defaultBinary
	}
	return p.FFprobe
}

func (p *Prober) timeout() time.Duration {
	if p == nil || p.Timeout <= 0 {
		return defaultTimeout
	}
	return p.Timeout
}

// inputArg converts a path into the ffprobe input argument. Local paths are
// always prefixed with the "file:" protocol so that names beginning with '-'
// are never parsed as options and names containing ':' are never parsed as
// another protocol. URLs (scheme://) are passed through unchanged.
func inputArg(path string) (string, error) {
	if path == "" {
		return "", errors.New("probe: empty path")
	}
	if strings.ContainsRune(path, 0) {
		return "", errors.New("probe: path contains NUL byte")
	}
	if strings.Contains(path, "://") {
		if strings.HasPrefix(path, "-") {
			return "", fmt.Errorf("probe: invalid input %q", path)
		}
		return path, nil
	}
	return "file:" + path, nil
}

// Probe runs ffprobe on path and returns the normalised description.
func (p *Prober) Probe(ctx context.Context, path string) (*Info, error) {
	in, err := inputArg(path)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(path, "://") {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("probe %s: %w", path, err)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	args := []string{
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters",
		in,
	}
	cmd := exec.CommandContext(ctx, p.binary(), args...)
	var stdout bytes.Buffer
	stderr := &limitedBuffer{max: maxStderr}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("probe %s: ffprobe interrupted: %w", path, ctxErr)
	}
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			if isNotMediaMessage(msg) {
				return nil, fmt.Errorf("probe %s: %s: %w", path, msg, ErrNotMedia)
			}
			return nil, fmt.Errorf("probe %s: ffprobe exited with code %d: %s", path, exitErr.ExitCode(), msg)
		}
		return nil, fmt.Errorf("probe %s: run ffprobe: %w", path, runErr)
	}

	info, err := ParseOutput(stdout.Bytes(), path)
	if err != nil {
		return nil, fmt.Errorf("probe %s: %w", path, err)
	}
	return info, nil
}

func isNotMediaMessage(msg string) bool {
	m := strings.ToLower(msg)
	for _, s := range []string{
		"invalid data found when processing input",
		"could not find codec parameters",
	} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// Version returns the ffprobe version, e.g. "6.1.1". Distribution suffixes
// ("6.1.1-3ubuntu5") are stripped; development builds ("N-113000-g…") are
// returned as-is.
func (p *Prober) Version(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()
	out, err := exec.CommandContext(ctx, p.binary(), "-hide_banner", "-version").Output()
	if err != nil {
		return "", fmt.Errorf("probe: ffprobe -version: %w", err)
	}
	v := parseVersion(string(out))
	if v == "" {
		return "", errors.New("probe: cannot parse ffprobe version output")
	}
	return v, nil
}

// parseVersion extracts the version from `ffprobe -version` output.
func parseVersion(out string) string {
	line, _, _ := strings.Cut(out, "\n")
	_, rest, ok := strings.Cut(line, "version ")
	if !ok {
		return ""
	}
	tok, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
	if tok == "" {
		return ""
	}
	if tok[0] >= '0' && tok[0] <= '9' {
		end := strings.IndexFunc(tok, func(r rune) bool {
			return (r < '0' || r > '9') && r != '.'
		})
		if end > 0 {
			tok = tok[:end]
		}
		tok = strings.TrimRight(tok, ".")
	}
	return tok
}

// limitedBuffer keeps at most max bytes and silently drops the rest.
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }
