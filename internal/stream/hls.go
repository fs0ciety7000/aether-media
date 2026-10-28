package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// errSegment is returned when FFmpeg cannot produce a segment.
var errSegment = errors.New("stream: segment unavailable")

// ServeHLS serves one HLS resource of session id: "master.m3u8",
// "main.m3u8", "init.mp4" or "<n>.m4s". Segments are produced on demand:
// a segment just ahead of FFmpeg is awaited, a segment behind the current
// run or far ahead of it restarts FFmpeg at that segment.
func (m *Manager) ServeHLS(w http.ResponseWriter, r *http.Request, id, name string) {
	s, ok := m.Get(id)
	if !ok || !s.HLS() {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	s.touch()
	switch {
	case name == "master.m3u8":
		writePlaylist(w, s.masterPlaylist())
	case name == "main.m3u8":
		writePlaylist(w, s.mediaPlaylist())
	case name == initName:
		data, err := s.waitInit(r.Context())
		if err != nil {
			s.httpError(w, err)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Cache-Control", "private, max-age=3600")
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
	case strings.HasSuffix(name, ".m4s"):
		n, err := strconv.Atoi(strings.TrimSuffix(name, ".m4s"))
		if err != nil || n < 0 || n >= s.plan.count() || strconv.Itoa(n)+".m4s" != name {
			http.Error(w, "segment not found", http.StatusNotFound)
			return
		}
		files, err := s.waitSegment(r.Context(), n)
		if err != nil {
			s.httpError(w, err)
			return
		}
		s.touch()
		serveFiles(w, r, files)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Session) httpError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "session not found", http.StatusNotFound)
	case errors.Is(err, context.Canceled):
		// client went away
	case errors.Is(err, context.DeadlineExceeded):
		http.Error(w, "segment not ready", http.StatusGatewayTimeout)
	default:
		s.m.log.Warn("hls request failed", "session", s.ID, "err", err)
		http.Error(w, "transcoding failed", http.StatusInternalServerError)
	}
}

func writePlaylist(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	io.WriteString(w, body)
}

// serveFiles writes the concatenation of files (one segment).
func serveFiles(w http.ResponseWriter, r *http.Request, files []string) {
	var size int64
	opened := make([]*os.File, 0, len(files))
	defer func() {
		for _, f := range opened {
			_ = f.Close()
		}
	}()
	for _, p := range files {
		f, err := os.Open(p)
		if err != nil {
			http.Error(w, "segment vanished", http.StatusNotFound)
			return
		}
		opened = append(opened, f)
		st, err := f.Stat()
		if err != nil {
			http.Error(w, "segment vanished", http.StatusNotFound)
			return
		}
		size += st.Size()
	}
	w.Header().Set("Content-Type", "video/iso.segment")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if r.Method == http.MethodHead {
		return
	}
	for _, f := range opened {
		if _, err := io.Copy(w, f); err != nil {
			return
		}
	}
}

// masterPlaylist has a single variant pointing at main.m3u8.
func (s *Session) masterPlaylist() string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	var codecs []string
	if c := videoCodecsString(s.Info, s.Decision); c != "" {
		codecs = append(codecs, c)
	}
	if c := audioCodecsString(s.Info, s.Decision); c != "" {
		codecs = append(codecs, c)
	}
	fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d", bandwidth(s.Info, s.Decision))
	if len(codecs) > 0 {
		fmt.Fprintf(&b, ",CODECS=\"%s\"", strings.Join(codecs, ","))
	}
	if w, h := outputSize(s.Info, s.Decision); w > 0 && h > 0 {
		fmt.Fprintf(&b, ",RESOLUTION=%dx%d", w, h)
	}
	if v := s.Info.Video(); v != nil && v.FrameRate > 0 && v.FrameRate <= 240 {
		fmt.Fprintf(&b, ",FRAME-RATE=%.3f", v.FrameRate)
	}
	if v := s.Info.Video(); v != nil && s.Decision.VideoCopy && isHDR(v) {
		if v.HDR == "HLG" || v.ColorTransfer == "arib-std-b67" {
			b.WriteString(",VIDEO-RANGE=HLG")
		} else {
			b.WriteString(",VIDEO-RANGE=PQ")
		}
	}
	b.WriteString("\nmain.m3u8\n")
	return b.String()
}

// mediaPlaylist is the full VOD playlist: every segment of the file is
// listed up front so the player knows the duration and can seek anywhere.
func (s *Session) mediaPlaylist() string {
	p := s.plan
	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n",
		int(math.Ceil(p.maxDuration()-1e-3)))
	b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"init.mp4\"\n")
	for n := range p.starts {
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n%d.m4s\n", p.duration(n), n)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

// waitInit returns the init segment, starting FFmpeg if no run exists.
func (s *Session) waitInit(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, segmentWait)
	defer cancel()
	for {
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return nil, ErrNotFound
		}
		if s.init != nil {
			data := s.init
			s.mu.Unlock()
			return data, nil
		}
		if p, ok := s.initFile(); ok {
			data, err := os.ReadFile(p)
			if err == nil {
				s.init = data
				s.mu.Unlock()
				return data, nil
			}
		}
		r := s.run
		if r == nil || r.exited() {
			if r != nil && r.err != nil && !r.killed {
				err := fmt.Errorf("%w: ffmpeg: %w: %s", errSegment, r.err, r.stderr.lastLines(5))
				s.mu.Unlock()
				return nil, err
			}
			if err := s.startRun(max(s.lastReq, 0)); err != nil {
				s.mu.Unlock()
				return nil, err
			}
		} else {
			s.resume(r)
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// waitSegment returns the files of complete segment n, waiting for the
// running FFmpeg when n is just ahead of it, or restarting FFmpeg at n when
// n is behind its start or too far ahead.
func (s *Session) waitSegment(ctx context.Context, n int) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, segmentWait)
	defer cancel()
	restarted := false
	for {
		s.verifyPending()
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return nil, ErrNotFound
		}
		s.lastReq = n
		if files, ok := s.findSegment(n); ok {
			if r := s.run; r != nil {
				s.resume(r)
			}
			s.mu.Unlock()
			return files, nil
		}
		r := s.run
		need := r == nil || r.failed != nil || r.exited() || n < r.startSeg
		if !need {
			produced := r.startSeg
			if r.verified {
				produced = s.advance(r)
			}
			need = n > produced+lookaheadSegments
		}
		switch {
		case need && restarted:
			// The run we started for n cannot produce it.
			if r == nil || r.exited() || r.failed != nil {
				err := fmt.Errorf("%w: segment %d", errSegment, n)
				if r != nil && r.failed != nil {
					err = fmt.Errorf("%w: %w", errSegment, r.failed)
				} else if r != nil && r.err != nil {
					err = fmt.Errorf("%w: ffmpeg: %w: %s", errSegment, r.err, r.stderr.lastLines(5))
				}
				s.mu.Unlock()
				return nil, err
			}
		case need:
			s.m.log.Debug("ffmpeg restart", "session", s.ID, "segment", n)
			if err := s.startRun(n); err != nil {
				s.mu.Unlock()
				return nil, err
			}
			restarted = true
		default:
			s.resume(r)
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
