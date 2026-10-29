package stream

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
)

// Errors returned by the Manager.
var (
	ErrBusy     = errors.New("stream: maximum number of concurrent transcodes reached")
	ErrNotFound = errors.New("stream: session not found")
)

// Config configures a Manager.
type Config struct {
	FFmpeg        string        // default "ffmpeg"
	FFprobe       string        // default: "ffprobe" next to FFmpeg (same directory), else from PATH
	TranscodeDir  string        // root for session dirs (default: <os temp>/aether-transcode)
	MaxTranscodes int           // concurrent ffmpeg sessions (remux counts too), default 2
	SegmentSec    int           // default 6
	IdleTimeout   time.Duration // default 60s (no ping and no segment request)
	HWAccel       string        // "none" (default) | "vaapi" | "qsv" | "nvenc" | "videotoolbox" | "auto"
	VAAPIDevice   string        // default /dev/dri/renderD128
	Keyframes     KeyframeStore // persistent keyframe cache (optional)
	Logger        *slog.Logger
}

// Tunables.
const (
	lookaheadSegments = 5                // wait instead of restarting when n ≤ produced+lookahead
	segmentWait       = 30 * time.Second // max wait for one segment
	defaultThrottle   = 2 * time.Minute  // pause ffmpeg when this far ahead of the player
	pollInterval      = 100 * time.Millisecond
)

// Manager owns the playback sessions and their FFmpeg processes.
type Manager struct {
	cfg     Config
	log     *slog.Logger
	kfCache *keyframeCache

	mu       sync.Mutex
	sessions map[string]*Session
	active   int // sessions with an FFmpeg pipeline (slots of MaxTranscodes)
	closed   bool

	capsOnce   sync.Once
	toneMapper toneMapper
	hw         string
	toneWarn   sync.Once // "no zscale" warning, once, when tone mapping is needed

	bgScans  map[string]bool // background keyframe scans in progress (guarded by mu)
	bgCtx    context.Context
	bgCancel context.CancelFunc

	throttleAhead time.Duration

	stop chan struct{}
	wg   sync.WaitGroup
}

// NewManager returns a Manager and starts its idle reaper.
func NewManager(cfg Config) *Manager {
	if cfg.FFmpeg == "" {
		cfg.FFmpeg = "ffmpeg"
	}
	if cfg.FFprobe == "" {
		cfg.FFprobe = "ffprobe"
		if dir := filepath.Dir(cfg.FFmpeg); dir != "." && strings.ContainsRune(cfg.FFmpeg, filepath.Separator) {
			name := strings.Replace(filepath.Base(cfg.FFmpeg), "ffmpeg", "ffprobe", 1)
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				cfg.FFprobe = filepath.Join(dir, name)
			}
		}
	}
	if cfg.TranscodeDir == "" {
		cfg.TranscodeDir = filepath.Join(os.TempDir(), "aether-transcode")
	}
	if abs, err := filepath.Abs(cfg.TranscodeDir); err == nil {
		cfg.TranscodeDir = abs
	}
	if cfg.MaxTranscodes <= 0 {
		cfg.MaxTranscodes = 2
	}
	if cfg.SegmentSec <= 0 {
		cfg.SegmentSec = 6
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 60 * time.Second
	}
	if cfg.HWAccel == "" {
		cfg.HWAccel = HWNone
	}
	if cfg.VAAPIDevice == "" {
		cfg.VAAPIDevice = "/dev/dri/renderD128"
	}
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	m := &Manager{
		cfg:      cfg,
		log:      log.With("component", "stream"),
		kfCache:  newKeyframeCache(128),
		sessions: map[string]*Session{},
		bgScans:  map[string]bool{},
		stop:     make(chan struct{}),

		throttleAhead: defaultThrottle,
	}
	m.bgCtx, m.bgCancel = context.WithCancel(context.Background())
	m.wg.Add(1)
	go m.reaper()
	return m
}

// caps detects (once) the FFmpeg build capabilities: zscale for tone
// mapping and, for HWAccel "auto", the first working hardware encoder.
func (m *Manager) caps() {
	m.capsOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		m.toneMapper = toneMapColorspace
		out, err := exec.CommandContext(ctx, m.cfg.FFmpeg, "-hide_banner", "-filters").Output()
		if err == nil && hasFilter(out, "zscale") && hasFilter(out, "tonemap") {
			m.toneMapper = toneMapZscale
		}
		m.hw = m.cfg.HWAccel
		if m.hw == HWAuto {
			m.hw = HWNone
			if encs := DetectHWAccel(ctx, m.cfg.FFmpeg); len(encs) > 0 {
				m.hw = hwModeOfEncoder(encs[0])
			}
			m.log.Info("hardware acceleration", "mode", m.hw)
		}
	})
}

func hasFilter(out []byte, name string) bool {
	for _, line := range bytes.Split(out, []byte("\n")) {
		f := bytes.Fields(line)
		if len(f) >= 2 && string(f[1]) == name {
			return true
		}
	}
	return false
}

// Session is one playback session. Exported fields are read-only.
type Session struct {
	ID         string
	Decision   Decision
	StartMs    int64 // actual start, aligned on a segment start for HLS
	DurationMs int64
	Path       string
	Info       *probe.Info
	Source     Source

	m          *Manager
	dir        string
	plan       *plan
	lastActive atomic.Int64 // unix nanos
	started    time.Time

	mu      sync.Mutex
	run     *run   // current process (nil when none)
	runs    []*run // every run, newest last (completed files stay usable)
	runSeq  int
	lastReq int
	init    []byte
	stopped bool
	hasSlot bool
	profile Profile
}

// HLS reports whether the session is served through HLS (remux/transcode).
func (s *Session) HLS() bool { return s.Decision.Method != MethodDirect }

// Segments returns the number of segments in the playlist (0 for direct).
func (s *Session) Segments() int {
	if s.plan == nil {
		return 0
	}
	return s.plan.count()
}

func (s *Session) touch() { s.lastActive.Store(time.Now().UnixNano()) }

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Start decides how to play r and, for remux/transcode, starts FFmpeg at
// the segment containing r.StartMs. Direct play creates no process but the
// session is registered for ping/stop bookkeeping. ErrBusy is returned when
// MaxTranscodes FFmpeg sessions are already running.
func (m *Manager) Start(ctx context.Context, r Request) (*Session, error) {
	if r.Info == nil {
		return nil, errors.New("stream: missing media info")
	}
	d := Decide(r)
	s := &Session{
		ID:         newID(),
		Decision:   d,
		StartMs:    max(r.StartMs, 0),
		DurationMs: r.Info.DurationMs,
		Path:       r.Path,
		Info:       r.Info,
		Source:     r.Source,
		m:          m,
		profile:    r.Profile,
		started:    time.Now(),
	}
	s.touch()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("stream: manager is shut down")
	}
	if d.Method == MethodDirect {
		m.sessions[s.ID] = s
		m.mu.Unlock()
		return s, nil
	}
	if m.active >= m.cfg.MaxTranscodes {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	m.active++
	s.hasSlot = true
	m.mu.Unlock()

	if err := m.prepare(ctx, s); err != nil {
		m.releaseSlot(s)
		if s.dir != "" {
			_ = os.RemoveAll(s.dir)
		}
		return nil, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		s.shutdown()
		m.releaseSlot(s)
		return nil, errors.New("stream: manager is shut down")
	}
	// Idle time counts from registration: preparing (keyframe scan, first
	// FFmpeg run) may take longer than IdleTimeout on a slow machine.
	s.touch()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	m.log.Info("playback session started", "session", s.ID, "method", d.Method, "reasons", d.Reasons,
		"videoCopy", d.VideoCopy, "audioCopy", d.AudioCopy, "toneMap", d.ToneMap, "startMs", s.StartMs)
	return s, nil
}

// prepare builds the segment plan and starts the first FFmpeg run.
func (m *Manager) prepare(ctx context.Context, s *Session) error {
	m.caps()
	if s.Path == "" {
		return errors.New("stream: missing media path")
	}
	total := float64(s.Info.DurationMs) / 1000
	seg := m.cfg.SegmentSec
	d := &s.Decision
	if d.VideoCopy {
		kf, err := m.keyframes(ctx, s.Path, s.Info, s.Source)
		var p *plan
		if err == nil {
			p, err = keyframePlan(kf, total, seg)
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Without keyframes a video copy cannot be segmented reliably.
			m.log.Warn("images clés indisponibles : la vidéo sera transcodée au lieu d'être copiée", "path", s.Path, "err", err)
			d.Method = MethodTranscode
			d.VideoCopy = false
			if !hasReason(d.Reasons, ReasonVideoCodec) {
				d.Reasons = append(d.Reasons, ReasonVideoCodec)
			}
			if v := s.Info.Video(); v != nil && isToneMappable(v) {
				d.ToneMap = true // H.264 output is SDR
			}
			d.TargetHeight, d.VideoBitrate = transcodeTarget(s.Info.Video(), streamByIndex(s.Info, d.AudioIndex), d.AudioCopy, s.profile)
		} else {
			s.plan = p
		}
	}
	if d.ToneMap && m.toneMapper == toneMapColorspace {
		m.toneWarn.Do(func() {
			m.log.Warn("FFmpeg sans zscale/tonemap : le tone mapping HDR → SDR se fait par une simple conversion d'espace colorimétrique (couleurs approximatives). Installez un FFmpeg compilé avec libzimg pour un rendu fidèle.")
		})
	}
	if s.plan == nil {
		if total <= 0 {
			return errors.New("stream: unknown media duration")
		}
		s.plan = uniformPlan(total, seg)
	}
	if s.DurationMs <= 0 {
		s.DurationMs = int64(s.plan.total * 1000)
	}
	start := s.plan.segmentAt(float64(s.StartMs) / 1000)
	s.StartMs = int64(s.plan.starts[start] * 1000)
	s.lastReq = start

	s.dir = filepath.Join(m.cfg.TranscodeDir, s.ID)
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("stream: create session dir: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startRun(start)
}

func (m *Manager) releaseSlot(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.hasSlot {
		s.hasSlot = false
		m.active--
	}
}

// Get returns a registered session.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Ping marks the session as alive; false when it does not exist.
func (m *Manager) Ping(id string) bool {
	s, ok := m.Get(id)
	if ok {
		s.touch()
	}
	return ok
}

// Stop kills the session's FFmpeg (if any) and removes its directory.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	s.shutdown()
	m.releaseSlot(s)
	m.log.Info("playback session stopped", "session", id)
}

// Shutdown stops every session and the reaper.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		close(m.stop)
		m.bgCancel()
	}
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Stop(id)
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// reaper stops sessions idle for longer than IdleTimeout.
func (m *Manager) reaper() {
	defer m.wg.Done()
	every := min(max(m.cfg.IdleTimeout/4, 20*time.Millisecond), 5*time.Second)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-t.C:
			var idle []string
			m.mu.Lock()
			for id, s := range m.sessions {
				if now.Sub(time.Unix(0, s.lastActive.Load())) > m.cfg.IdleTimeout {
					idle = append(idle, id)
				}
			}
			m.mu.Unlock()
			for _, id := range idle {
				m.log.Info("playback session idle, stopping", "session", id)
				m.Stop(id)
			}
		}
	}
}

// shutdown kills the current run and removes the session directory.
func (s *Session) shutdown() {
	s.mu.Lock()
	s.stopped = true
	r := s.run
	s.run = nil
	s.mu.Unlock()
	if r != nil {
		r.kill()
	}
	if s.dir != "" {
		if err := os.RemoveAll(s.dir); err != nil {
			s.m.log.Warn("remove session dir", "dir", s.dir, "err", err)
		}
	}
}
