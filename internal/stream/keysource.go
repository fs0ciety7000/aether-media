package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/media/mkv"
	"github.com/fs0ciety7000/aether-media/internal/media/probe"
)

// Source tells the Manager where a file comes from, so that it can find
// its keyframes without reading it whole (T-068).
type Source struct {
	// MediaFileID and Version identify this version of the file across
	// sessions and restarts (persistent keyframe cache); Version changes
	// with the file (e.g. "<size>:<mtime>"). Zero ID: no persistent cache.
	MediaFileID int64
	Version     string
	// Open opens the file for ranged reads (Matroska Cues index). nil: no
	// direct access, only FFmpeg's input (Request.Path) is used.
	Open func(ctx context.Context) (io.ReadSeekCloser, int64, error)
	// Remote: reading the whole file is slow (agent, SFTP across the
	// Internet). The keyframes are then never obtained by an ffprobe packet
	// scan, which reads every byte of the file.
	Remote bool
}

// KeyframeStore persists keyframe lists (the server's database).
type KeyframeStore interface {
	LoadKeyframes(ctx context.Context, mediaFileID int64, version string) ([]float64, bool)
	SaveKeyframes(ctx context.Context, mediaFileID int64, version string, kf []float64)
}

// Keyframe acquisition budgets. The whole lookup is bounded by
// keyframeBudget so that a session starts (or falls back to a transcode)
// in a bounded time, well below the player's own start timeout.
const (
	keyframeBudget = 40 * time.Second
	// indexTimeout bounds the Matroska index read (a few ranged reads).
	indexTimeout = 25 * time.Second
	// backgroundScanTimeout bounds the ffprobe scan of a local file that
	// did not finish within keyframeBudget: it goes on in the background so
	// that the next session can remux.
	backgroundScanTimeout = 20 * time.Minute
)

// errNoRemoteIndex: a remote file without a usable index is not scanned.
var errNoRemoteIndex = errors.New("pas d'index des images clés exploitable (fichier distant : il n'est pas lu en entier)")

// keyframeCacheKey identifies a file version for the in-memory cache: the
// media file id when known (the FFmpeg input of a remote file is a signed
// URL that changes with each session), else path + size + duration.
func keyframeCacheKey(path string, info *probe.Info, src Source) string {
	if src.MediaFileID > 0 {
		return fmt.Sprintf("mf:%d:%s", src.MediaFileID, src.Version)
	}
	return kfKey(path, info)
}

// keyframes returns the keyframe list of the session's file: memory cache,
// database, Matroska index (a few ranged reads), and for local files only
// an ffprobe packet scan.
func (m *Manager) keyframes(ctx context.Context, path string, info *probe.Info, src Source) (keyframes, error) {
	key := keyframeCacheKey(path, info, src)
	if kf, ok := m.kfCache.get(key); ok {
		return kf, nil
	}
	if m.cfg.Keyframes != nil && src.MediaFileID > 0 {
		if kf, ok := m.cfg.Keyframes.LoadKeyframes(ctx, src.MediaFileID, src.Version); ok && len(kf) > 0 {
			m.kfCache.put(key, kf)
			return kf, nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, keyframeBudget)
	defer cancel()

	var indexErr error
	if src.Open != nil && hasMatroskaIndex(info.Container) {
		kf, err := m.readIndex(ctx, path, info, src)
		if err == nil {
			m.remember(key, src, kf)
			return kf, nil
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, ctx.Err() // the client went away
		}
		indexErr = err
	}
	if src.Remote {
		if indexErr != nil {
			return nil, fmt.Errorf("%w : %w", errNoRemoteIndex, indexErr)
		}
		return nil, errNoRemoteIndex
	}

	t := time.Now()
	kf, err := scanKeyframes(ctx, m.cfg.FFprobe, path, keyframeBudget)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			m.scanInBackground(key, path, src)
		}
		return nil, err
	}
	m.log.Debug("keyframes scanned", "path", path, "count", len(kf), "took", time.Since(t))
	m.remember(key, src, kf)
	return kf, nil
}

func hasMatroskaIndex(container string) bool {
	switch container {
	case "mkv", "webm", "matroska":
		return true
	}
	return false
}

// readIndex reads the Matroska Cues of the file and shifts them onto the
// zero-based timeline FFmpeg uses with -start_at_zero.
func (m *Manager) readIndex(ctx context.Context, path string, info *probe.Info, src Source) (keyframes, error) {
	ctx, cancel := context.WithTimeout(ctx, indexTimeout)
	defer cancel()
	t := time.Now()
	f, size, err := src.Open(ctx)
	if err != nil {
		return nil, err
	}
	type result struct {
		idx *mkv.Index
		err error
	}
	ch := make(chan result, 1)
	go func() {
		// The file is closed by the reader (never concurrently with a read).
		idx, err := mkv.ReadIndex(f, size)
		_ = f.Close()
		ch <- result{idx, err}
	}()
	var res result
	select {
	case res = <-ch:
	case <-ctx.Done():
		// Reads of a remote file honour ctx (it was opened with it); a
		// local read finishes on its own. Either way do not wait.
		return nil, ctx.Err()
	}
	if res.err != nil {
		m.log.Info("index des images clés inutilisable", "path", path, "err", res.err)
		return nil, res.err
	}
	kf := make(keyframes, 0, len(res.idx.Times))
	for _, t := range res.idx.Times {
		kf = append(kf, t-info.StartTime)
	}
	sort.Float64s(kf)
	m.log.Info("index des images clés lu", "path", path, "count", len(kf),
		"reads", res.idx.Reads, "bytes", res.idx.Bytes, "took", time.Since(t).Round(time.Millisecond))
	return kf, nil
}

func (m *Manager) remember(key string, src Source, kf keyframes) {
	m.kfCache.put(key, kf)
	if m.cfg.Keyframes != nil && src.MediaFileID > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		m.cfg.Keyframes.SaveKeyframes(ctx, src.MediaFileID, src.Version, kf)
	}
}

// scanInBackground goes on scanning a local file whose scan did not fit in
// the start budget (the session falls back to a transcode meanwhile), once
// per file version.
func (m *Manager) scanInBackground(key, path string, src Source) {
	m.mu.Lock()
	if m.closed || m.bgScans[key] {
		m.mu.Unlock()
		return
	}
	m.bgScans[key] = true
	m.wg.Add(1)
	m.mu.Unlock()
	m.log.Info("analyse des images clés poursuivie en arrière-plan (la prochaine lecture pourra éviter le transcodage)", "path", path)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.mu.Lock()
			delete(m.bgScans, key)
			m.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(m.bgCtx, backgroundScanTimeout)
		defer cancel()
		t := time.Now()
		kf, err := scanKeyframes(ctx, m.cfg.FFprobe, path, backgroundScanTimeout)
		if err != nil {
			if m.bgCtx.Err() == nil {
				m.log.Warn("analyse des images clés en arrière-plan impossible", "path", path, "err", err)
			}
			return
		}
		m.remember(key, src, kf)
		m.log.Info("images clés analysées en arrière-plan", "path", path, "count", len(kf), "took", time.Since(t).Round(time.Second))
	}()
}
