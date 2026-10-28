package stream

import (
	"strings"
	"sync"
)

// inputArg turns a local path into an FFmpeg input argument: the "file:"
// protocol prefix keeps names starting with '-' from being parsed as options
// and names containing ':' from being parsed as another protocol.
func inputArg(path string) string {
	if strings.Contains(path, "://") && !strings.HasPrefix(path, "-") {
		return path
	}
	return "file:" + path
}

// isURL reports whether an input is a network URL (http/https sources
// served by a remote agent or an internal proxy, with Range support).
func isURL(path string) bool {
	l := strings.ToLower(path)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// inputOptions are the demuxer/protocol options placed before -i.
// Request.Path is an opaque FFmpeg input: it is never stat'ed or opened by
// this package. HTTP inputs reconnect on errors; -ss before -i still seeks
// through HTTP Range requests.
func inputOptions(path string) []string {
	if !isURL(path) {
		return nil
	}
	return []string{"-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_on_network_error", "1", "-reconnect_delay_max", "5"}
}

// tailBuffer is an io.Writer keeping only the last max bytes written
// (FFmpeg stderr). Safe for concurrent use.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	limit := t.max
	if limit <= 0 {
		limit = 8 << 10
	}
	t.b = append(t.b, p...)
	if over := len(t.b) - limit; over > 0 {
		t.b = append(t.b[:0], t.b[over:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}

// lastLines returns the last n non-empty lines.
func (t *tailBuffer) lastLines(n int) string {
	lines := strings.Split(strings.TrimSpace(t.String()), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// escapeFilterValue escapes a value for an FFmpeg filter option
// (first escaping level: backslash, single quote and colon).
func escapeFilterValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '\'', ':':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeFilterGraph escapes a filter description for a filter graph
// (second escaping level: backslash, single quote, brackets, comma, semicolon).
func escapeFilterGraph(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '\'', '[', ']', ',', ';':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeSegmentPattern escapes '%' in a directory used in an FFmpeg output
// pattern (-hls_segment_filename expands %d).
func escapeSegmentPattern(s string) string { return strings.ReplaceAll(s, "%", "%%") }
