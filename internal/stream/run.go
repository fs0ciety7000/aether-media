package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// run is one FFmpeg process of a session, producing segments from
// startSeg onwards into its own directory (so a restart never races with
// files of a killed process).
type run struct {
	id         int
	dir        string
	startSeg   int
	startChunk int // ffmpeg -start_number (keyframe chunk index for video copy)
	keyframed  bool

	cmd    *exec.Cmd
	stderr *tailBuffer
	done   chan struct{}
	err    error // valid after done

	// Guarded by Session.mu.
	verified bool // output numbering checked against the plan (video copy)
	shift    int  // actual chunk index - file number
	next     int  // first segment not yet complete in this run
	paused   bool
	killed   bool
	failed   error // verification failure

	killOnce sync.Once
	verifyMu sync.Mutex // serialises verifyRun
}

func (r *run) exited() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// kill terminates the process (resuming it first if paused) and waits.
func (r *run) kill() {
	r.killOnce.Do(func() {
		if r.cmd.Process != nil {
			_ = resumeProcess(r.cmd.Process)
			_ = r.cmd.Process.Kill()
		}
	})
	select {
	case <-r.done:
	case <-time.After(10 * time.Second):
	}
}

// startRun starts FFmpeg at playlist segment n. Caller holds s.mu.
func (s *Session) startRun(n int) error {
	if s.stopped {
		return ErrNotFound
	}
	if old := s.run; old != nil {
		old.killed = true
		s.run = nil
		old.kill()
	}
	m := s.m
	s.runSeq++
	r := &run{
		id:        s.runSeq,
		dir:       filepath.Join(s.dir, "r"+strconv.Itoa(s.runSeq)),
		startSeg:  n,
		keyframed: s.plan.keyframed(),
		stderr:    &tailBuffer{max: 8 << 10},
		done:      make(chan struct{}),
		next:      n,
	}
	if r.keyframed {
		r.startChunk = s.plan.chunkStart[n]
	} else {
		r.startChunk = n
	}
	// Runs starting at the very beginning do not seek: numbering is exact.
	r.verified = !r.keyframed || n == 0
	if err := os.MkdirAll(r.dir, 0o750); err != nil {
		return fmt.Errorf("stream: create run dir: %w", err)
	}
	args := buildArgs(jobParams{
		Input:       s.Path,
		Info:        s.Info,
		Decision:    s.Decision,
		HW:          m.hw,
		VAAPIDevice: m.cfg.VAAPIDevice,
		ToneMapper:  m.toneMapper,
		SegmentSec:  m.cfg.SegmentSec,
		Dir:         r.dir,
		Plan:        s.plan,
		StartSeg:    n,
	})
	cmd := exec.Command(m.cfg.FFmpeg, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = r.stderr
	cmd.Dir = r.dir
	r.cmd = cmd
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("stream: start ffmpeg: %w", err)
	}
	m.log.Debug("ffmpeg started", "session", s.ID, "run", r.id, "segment", n, "args", args)
	go func() {
		r.err = cmd.Wait()
		close(r.done)
	}()
	s.run = r
	s.runs = append(s.runs, r)
	m.wg.Add(1)
	go s.monitor(r)
	return nil
}

// monitor verifies the output numbering of seeking video-copy runs,
// throttles FFmpeg when it is far ahead of the player, and logs failures.
func (s *Session) monitor(r *run) {
	defer s.m.wg.Done()
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-r.done:
			s.mu.Lock()
			killed := r.killed || s.stopped
			s.mu.Unlock()
			if r.err != nil && !killed {
				s.m.log.Warn("ffmpeg failed", "session", s.ID, "run", r.id, "err", r.err, "stderr", r.stderr.lastLines(10))
			}
			return
		case <-s.m.stop:
			return
		case <-t.C:
		}
		s.mu.Lock()
		if s.stopped || r.killed {
			s.mu.Unlock()
			return
		}
		needVerify := !r.verified && r.failed == nil
		s.mu.Unlock()
		if needVerify {
			s.verifyRun(r)
		}
		s.throttle(r)
	}
}

// verifyRun checks which keyframe a seeking video-copy run really started
// at (demuxer seeks are not always exact) by probing its first chunk, and
// records the numbering shift.
func (s *Session) verifyRun(r *run) {
	r.verifyMu.Lock()
	defer r.verifyMu.Unlock()
	s.mu.Lock()
	done := r.verified || r.failed != nil
	s.mu.Unlock()
	if done {
		return
	}
	initPath := filepath.Join(r.dir, initName)
	first := filepath.Join(r.dir, segmentFile(true, r.startChunk))
	if _, err := os.Stat(first); err != nil {
		return
	}
	pts, err := firstVideoPTS(s.m.cfg.FFprobe, initPath, first, filepath.Join(r.dir, "verify.mp4"))
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		r.failed = err
		s.m.log.Warn("cannot verify remux start", "session", s.ID, "err", err)
		return
	}
	kf := s.plan.kf
	best, bestD := -1, math.Inf(1)
	for i, t := range kf {
		if d := math.Abs(t - pts); d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 || bestD > 0.25 {
		r.failed = fmt.Errorf("stream: remux started at %.3fs, not on a known keyframe", pts)
		s.m.log.Warn("remux start mismatch", "session", s.ID, "pts", pts)
		return
	}
	r.shift = best - r.startChunk
	r.verified = true
	if r.shift != 0 {
		s.m.log.Debug("remux run started off-target", "session", s.ID, "shift", r.shift)
	}
}

// verifyPending verifies every run whose numbering is still unchecked
// (a fast remux may finish before its monitor ticks).
func (s *Session) verifyPending() {
	s.mu.Lock()
	var todo []*run
	for _, r := range s.runs {
		if !r.verified && r.failed == nil {
			todo = append(todo, r)
		}
	}
	s.mu.Unlock()
	for _, r := range todo {
		s.verifyRun(r)
	}
}

// firstVideoPTS returns the first video packet pts of init+segment.
func firstVideoPTS(ffprobe, initPath, segPath, tmp string) (float64, error) {
	if err := concatFiles(tmp, initPath, segPath); err != nil {
		return 0, err
	}
	defer func() { _ = os.Remove(tmp) }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time", "-read_intervals", "%+#1", "-of", "csv=p=0", inputArg(tmp)).Output()
	if err != nil {
		return 0, fmt.Errorf("stream: probe segment: %w", err)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(line, ",")), 64)
	if err != nil {
		return 0, fmt.Errorf("stream: parse segment pts %q: %w", line, err)
	}
	return v, nil
}

func concatFiles(dst string, srcs ...string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	for _, p := range srcs {
		in, err := os.Open(p)
		if err != nil {
			_ = f.Close()
			return err
		}
		_, err = io.Copy(f, in)
		_ = in.Close()
		if err != nil {
			_ = f.Close()
			return err
		}
	}
	return f.Close()
}

// throttle pauses FFmpeg when it is more than throttleAhead ahead of the
// last requested segment, and resumes it when the player catches up.
func (s *Session) throttle(r *run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != r || r.exited() {
		return
	}
	produced := s.advance(r)
	ahead := 0.0
	if produced > s.lastReq && s.lastReq >= 0 && produced <= s.plan.count() {
		ahead = s.plan.starts[min(produced, s.plan.count()-1)] - s.plan.starts[s.lastReq]
		if produced == s.plan.count() {
			ahead = s.plan.total - s.plan.starts[s.lastReq]
		}
	}
	limit := s.m.throttleAhead.Seconds()
	switch {
	case !r.paused && ahead > limit:
		if err := pauseProcess(r.cmd.Process); err == nil {
			r.paused = true
			s.m.log.Debug("ffmpeg paused", "session", s.ID, "aheadSec", ahead)
		}
	case r.paused && ahead < limit/2:
		s.resume(r)
	}
}

func (s *Session) resume(r *run) {
	if r.paused {
		if err := resumeProcess(r.cmd.Process); err == nil || errors.Is(err, os.ErrProcessDone) {
			r.paused = false
		}
	}
}

// advance moves r.next past the segments run r has completed and returns
// it (the segment FFmpeg is currently producing). Caller holds s.mu.
func (s *Session) advance(r *run) int {
	if !r.verified {
		return r.next
	}
	for r.next < s.plan.count() {
		if _, ok := s.segmentFilesIn(r, r.next); !ok {
			break
		}
		r.next++
	}
	return r.next
}

// segmentFilesIn returns the files making up segment n in run r if they
// are all complete. FFmpeg writes "<name>.tmp" and renames on completion
// (hls_flags temp_file), so an existing final name is a complete file.
func (s *Session) segmentFilesIn(r *run, n int) ([]string, bool) {
	if n < r.startSeg || !r.verified {
		return nil, false
	}
	if !r.keyframed {
		p := filepath.Join(r.dir, segmentFile(false, n))
		if _, err := os.Stat(p); err != nil {
			return nil, false
		}
		return []string{p}, true
	}
	from, to := s.plan.chunkStart[n], s.plan.chunkStart[n+1]
	files := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		num := i - r.shift
		if num < r.startChunk {
			return nil, false
		}
		p := filepath.Join(r.dir, segmentFile(true, num))
		if _, err := os.Stat(p); err != nil {
			return nil, false
		}
		files = append(files, p)
	}
	return files, true
}

// findSegment looks for a complete segment n in any run, newest first.
// Caller holds s.mu.
func (s *Session) findSegment(n int) ([]string, bool) {
	for i := len(s.runs) - 1; i >= 0; i-- {
		if files, ok := s.segmentFilesIn(s.runs[i], n); ok {
			return files, true
		}
	}
	return nil, false
}

// initFile returns the path of a complete init.mp4, if any run has one.
// The init segment is written before the first media segment is renamed,
// so it is complete once that segment exists (or the process ended well).
func (s *Session) initFile() (string, bool) {
	for i := len(s.runs) - 1; i >= 0; i-- {
		r := s.runs[i]
		p := filepath.Join(r.dir, initName)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		first := filepath.Join(r.dir, segmentFile(r.keyframed, r.startChunk))
		if _, err := os.Stat(first); err == nil {
			return p, true
		}
		if r.exited() && r.err == nil {
			return p, true
		}
	}
	return "", false
}
