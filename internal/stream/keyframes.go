package stream

import (
	"bufio"
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
)

// keyframes are the video keyframe times of a file, in seconds on the
// "zero-based" timeline FFmpeg uses with -start_at_zero (packet pts minus
// the container start time), sorted ascending.
type keyframes []float64

// scanKeyframes lists the keyframes of the first video stream with a
// packet scan (no decoding). It reads the whole file: used for local files
// only, bounded by timeout.
func scanKeyframes(ctx context.Context, ffprobe, path string, timeout time.Duration) (keyframes, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"-v", "error"}
	args = append(args, inputOptions(path)...)
	args = append(args,
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags:format=start_time",
		"-of", "csv",
		inputArg(path))
	cmd := exec.CommandContext(ctx, ffprobe, args...)
	var stderr tailBuffer
	stderr.max = 2 << 10
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("stream: keyframe scan: %w", ctx.Err())
		}
		return nil, fmt.Errorf("stream: keyframe scan: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseKeyframes(out)
}

// parseKeyframes parses `ffprobe -show_entries packet=pts_time,flags:format=start_time -of csv`.
func parseKeyframes(out []byte) (keyframes, error) {
	var kf keyframes
	start := 0.0
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		f := strings.Split(strings.TrimSpace(sc.Text()), ",")
		switch {
		case len(f) >= 3 && f[0] == "packet":
			if !strings.HasPrefix(f[2], "K") {
				continue
			}
			t, err := strconv.ParseFloat(f[1], 64)
			if err != nil || math.IsNaN(t) || math.IsInf(t, 0) {
				continue
			}
			kf = append(kf, t)
		case len(f) >= 2 && f[0] == "format":
			if t, err := strconv.ParseFloat(f[1], 64); err == nil && !math.IsNaN(t) && !math.IsInf(t, 0) {
				start = t
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(kf) == 0 {
		return nil, errors.New("stream: no video keyframe found")
	}
	for i := range kf {
		kf[i] -= start
	}
	sort.Float64s(kf)
	out2 := kf[:1]
	for _, t := range kf[1:] {
		if t-out2[len(out2)-1] > 1e-6 {
			out2 = append(out2, t)
		}
	}
	return out2, nil
}

// keyframeCache is a small in-memory LRU of keyframe lists (see
// keyframeCacheKey); the database keeps them across restarts.
type keyframeCache struct {
	mu    sync.Mutex
	max   int
	ll    *list.List
	items map[string]*list.Element
}

type kfEntry struct {
	key string
	kf  keyframes
}

func newKeyframeCache(maxEntries int) *keyframeCache {
	return &keyframeCache{max: maxEntries, ll: list.New(), items: map[string]*list.Element{}}
}

// kfKey identifies a file version without touching it (the path may be a
// remote URL): path + probed size + duration.
func kfKey(path string, info *probe.Info) string {
	var size, dur int64
	if info != nil {
		size, dur = info.Size, info.DurationMs
	}
	return fmt.Sprintf("%s\x00%d\x00%d", path, size, dur)
}

func (c *keyframeCache) get(key string) (keyframes, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.ll.MoveToFront(e)
		return e.Value.(*kfEntry).kf, true
	}
	return nil, false
}

func (c *keyframeCache) put(key string, kf keyframes) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		e.Value.(*kfEntry).kf = kf
		c.ll.MoveToFront(e)
		return
	}
	c.items[key] = c.ll.PushFront(&kfEntry{key: key, kf: kf})
	for c.ll.Len() > c.max {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.items, last.Value.(*kfEntry).key)
	}
}

// plan is the fixed segment layout of a session, advertised up front in
// main.m3u8.
type plan struct {
	starts []float64 // segment start times (s); starts[0] == 0
	total  float64   // media duration (s)

	// Keyframe-aligned (video copy) plans only: ffmpeg cuts one "chunk"
	// per source keyframe; segment n is the concatenation of chunks
	// chunkStart[n] .. chunkStart[n+1]-1 (chunk i begins at kf[i]).
	kf         keyframes
	chunkStart []int // len(starts)+1
}

func (p *plan) count() int { return len(p.starts) }

func (p *plan) duration(n int) float64 {
	if n+1 < len(p.starts) {
		return p.starts[n+1] - p.starts[n]
	}
	return p.total - p.starts[n]
}

func (p *plan) maxDuration() float64 {
	m := 0.0
	for n := range p.starts {
		m = math.Max(m, p.duration(n))
	}
	return m
}

// segmentAt returns the segment containing t seconds.
func (p *plan) segmentAt(t float64) int {
	n := sort.Search(len(p.starts), func(i int) bool { return p.starts[i] > t }) - 1
	return max(n, 0)
}

func (p *plan) keyframed() bool { return p.kf != nil }

// uniformPlan: fixed seg-second segments (transcode, forced keyframes).
func uniformPlan(total float64, seg int) *plan {
	if total <= 0 {
		total = float64(seg)
	}
	// A tail shorter than minLastSegment is merged into the last segment
	// (it usually holds no video keyframe, so FFmpeg would not cut there).
	n := max(int(math.Ceil((total-minLastSegment)/float64(seg))), 1)
	p := &plan{total: total, starts: make([]float64, n)}
	for i := range p.starts {
		p.starts[i] = float64(i * seg)
	}
	return p
}

// Keyframe-plan tuning.
const (
	// minBoundaryGap: a keyframe only becomes a segment boundary if the
	// next keyframe is at least this far, so that a restart can seek
	// slightly past it (seekPastKeyframe) and still land on it.
	minBoundaryGap = 0.5
	// seekPastKeyframe is added to the boundary keyframe time for -ss:
	// FFmpeg rewinds non-SEEK_TO_PTS demuxers (MKV) by 3/23 s when the
	// video has B-frames, and seeks to the keyframe at or before the target.
	seekPastKeyframe = 0.2
	minLastSegment   = 0.25
)

// keyframePlan groups source keyframes into ~seg-second segments for video
// copy. Boundaries are the first eligible keyframe at or after each
// multiple of seg (cumulative grid, so long GOPs do not drift the layout).
func keyframePlan(kf keyframes, total float64, seg int) (*plan, error) {
	if total <= 0 && len(kf) > 0 {
		total = kf[len(kf)-1] + float64(seg)
	}
	var k keyframes
	for _, t := range kf {
		if t < total {
			k = append(k, t)
		}
	}
	if len(k) == 0 {
		return nil, errors.New("stream: no keyframe within duration")
	}
	p := &plan{total: total, kf: k, starts: []float64{0}, chunkStart: []int{0}}
	desired := float64(seg)
	for i := 1; i < len(k); i++ {
		t := k[i]
		if t < desired || total-t < minLastSegment {
			continue
		}
		if i+1 < len(k) && k[i+1]-t < minBoundaryGap {
			continue
		}
		p.starts = append(p.starts, t)
		p.chunkStart = append(p.chunkStart, i)
		for desired <= t {
			desired += float64(seg)
		}
	}
	p.chunkStart = append(p.chunkStart, len(k))
	return p, nil
}
