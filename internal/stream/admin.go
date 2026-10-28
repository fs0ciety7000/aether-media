package stream

import (
	"sort"
	"time"
)

// SessionInfo is a read-only snapshot of a playback session (admin view).
type SessionInfo struct {
	ID         string
	Decision   Decision
	StartMs    int64
	DurationMs int64
	Started    time.Time
	LastActive time.Time
	// Bitrate is the estimated output bitrate (bits/s): the source's for
	// direct play and copies, the encoder target for transcodes.
	Bitrate int64
	// HW is the hardware acceleration used for a video transcode ("" when
	// the video is not encoded, "none" for software).
	HW         string
	VideoCodec string
	AudioCodec string
	Width      int
	Height     int
}

// Sessions returns the registered sessions, oldest first.
func (m *Manager) Sessions() []SessionInfo {
	m.mu.Lock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.Unlock()
	out := make([]SessionInfo, 0, len(list))
	for _, s := range list {
		out = append(out, s.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.Before(out[j].Started) })
	return out
}

func (s *Session) snapshot() SessionInfo {
	d := s.Decision
	si := SessionInfo{
		ID: s.ID, Decision: d, StartMs: s.StartMs, DurationMs: s.DurationMs,
		Started: s.started, LastActive: time.Unix(0, s.lastActive.Load()),
	}
	var src int64
	if s.Info != nil {
		src = s.Info.Bitrate
		if v := s.Info.Video(); v != nil {
			si.VideoCodec, si.Width, si.Height = v.Codec, v.Width, v.Height
		}
		if a := streamByIndex(s.Info, d.AudioIndex); a != nil {
			si.AudioCodec = a.Codec
		}
	}
	switch {
	case d.Method == MethodDirect || (d.VideoCopy && d.AudioCopy):
		si.Bitrate = src
	default:
		if d.VideoCopy {
			si.Bitrate = src
		} else {
			si.Bitrate = d.VideoBitrate
			si.HW = s.m.HWAccel()
			if si.HW == "" {
				si.HW = HWNone
			}
			if d.TargetHeight > 0 && si.Height > 0 {
				si.Width = si.Width * d.TargetHeight / si.Height
				si.Height = d.TargetHeight
			}
		}
		if !d.AudioCopy && d.AudioIndex >= 0 {
			si.Bitrate += audioTranscodeBitrate
		}
	}
	return si
}

// SetMaxTranscodes changes the number of concurrent FFmpeg sessions for
// new sessions (running ones are never interrupted).
func (m *Manager) SetMaxTranscodes(n int) {
	if n < 1 {
		return
	}
	m.mu.Lock()
	m.cfg.MaxTranscodes = n
	m.mu.Unlock()
}

// Limits returns the configured maximum and the FFmpeg sessions in use.
func (m *Manager) Limits() (maxTranscodes, active int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.MaxTranscodes, m.active
}

// HWAccel returns the hardware acceleration in use ("none" for software),
// detecting it first when configured as "auto".
func (m *Manager) HWAccel() string {
	m.caps()
	return m.hw
}
