// Package mkv reads the keyframe index (Cues) of Matroska / WebM files.
//
// Remuxing a video stream into HLS needs the exact list of its keyframes.
// Asking ffprobe for it demuxes every packet, i.e. reads the whole file: a
// 20-40 GB Blu-ray remux on a remote agent is minutes of transfer before the
// first image. A Matroska file normally carries that list in its Cues
// element (mkvmerge and FFmpeg write one cue point per video keyframe), a
// few hundred KiB near the end of the file, located by the SeekHead at its
// start. This package reads it with a handful of ranged reads and checks it
// against real cluster samples, so that an incomplete index is rejected
// instead of producing misaligned segments.
package mkv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
)

// Errors.
var (
	// ErrNotMatroska: the file does not start with an EBML/Matroska header.
	ErrNotMatroska = errors.New("mkv: not a Matroska file")
	// ErrNoCues: the file has no usable Cues element (or no video track).
	ErrNoCues = errors.New("mkv: no keyframe index (Cues)")
	// ErrIncompleteCues: a sampled cluster holds a video keyframe that is
	// not listed in the Cues (index written for some keyframes only).
	ErrIncompleteCues = errors.New("mkv: keyframe index is incomplete")
)

// Element IDs (with their length marker, as written in the file).
const (
	idEBML           = 0x1A45DFA3
	idSegment        = 0x18538067
	idSeekHead       = 0x114D9B74
	idSeek           = 0x4DBB
	idSeekID         = 0x53AB
	idSeekPosition   = 0x53AC
	idInfo           = 0x1549A966
	idTimecodeScale  = 0x2AD7B1
	idTracks         = 0x1654AE6B
	idTrackEntry     = 0xAE
	idTrackNumber    = 0xD7
	idTrackType      = 0x83
	idCues           = 0x1C53BB6B
	idCuePoint       = 0xBB
	idCueTime        = 0xB3
	idCueTrackPos    = 0xB7
	idCueTrack       = 0xF7
	idCueClusterPos  = 0xF1
	idCluster        = 0x1F43B675
	idClusterTime    = 0xE7
	idSimpleBlock    = 0xA3
	idBlockGroup     = 0xA0
	idBlock          = 0xA1
	idReferenceBlock = 0xFB

	trackTypeVideo = 1
)

// Read limits: the index of a feature film is a few hundred KiB; anything
// far larger is not worth reading through a slow link.
const (
	headRead     = 64 << 10
	maxMetaRead  = 4 << 20  // Info, Tracks, SeekHead
	maxCuesRead  = 32 << 20 // Cues
	sampleWindow = 2 << 20  // bytes of a cluster checked against the index
	maxTopLevel  = 64       // top-level elements inspected before the first cluster
)

// Index is the keyframe index of the first video track.
type Index struct {
	// Times are the cue point timestamps of the video track in seconds, on
	// the file's own timeline (the packet timestamps ffprobe reports),
	// sorted and de-duplicated.
	Times []float64
	// Reads is the number of ranged reads issued, Bytes their total size.
	Reads int
	Bytes int64
}

type reader struct {
	r        io.ReadSeeker
	noReadAt bool
	size     int64
	reads    int
	bytes    int64
}

// readAt reads up to n bytes at off (fewer at the end of the file).
func (rd *reader) readAt(off, n int64) ([]byte, error) {
	if off < 0 || off >= rd.size {
		return nil, fmt.Errorf("mkv: offset %d outside file (%d bytes)", off, rd.size)
	}
	n = min(n, rd.size-off)
	buf := make([]byte, n)
	var got int
	var err error
	if ra, ok := rd.r.(io.ReaderAt); ok && !rd.noReadAt {
		// One bounded request per read on remote files.
		got, err = ra.ReadAt(buf, off)
		if errors.Is(err, errors.ErrUnsupported) {
			rd.noReadAt = true
			return rd.readAt(off, n)
		}
		if errors.Is(err, io.EOF) && got == len(buf) {
			err = nil
		}
	} else {
		if _, err := rd.r.Seek(off, io.SeekStart); err != nil {
			return nil, err
		}
		got, err = io.ReadFull(rd.r, buf)
	}
	rd.reads++
	rd.bytes += int64(got)
	short := errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
	if err != nil && (!short || got == 0) {
		return nil, err
	}
	return buf[:got], nil
}

// ReadIndex reads and checks the keyframe index of the first video track.
// r is read with a few seeks (or bounded ReadAt calls when it implements
// io.ReaderAt); size is the file size.
func ReadIndex(r io.ReadSeeker, size int64) (*Index, error) {
	rd := &reader{r: r, size: size}
	idx, err := rd.index()
	if idx == nil {
		idx = &Index{}
	}
	idx.Reads, idx.Bytes = rd.reads, rd.bytes
	if err != nil {
		return idx, err
	}
	return idx, nil
}

// segmentMeta is what the top of the segment tells us.
type segmentMeta struct {
	start        int64 // offset of the Segment payload (positions are relative to it)
	end          int64 // end of the Segment payload (size when unknown)
	scale        uint64
	videoTrack   uint64
	cuesPos      int64 // absolute, -1 if unknown
	infoPos      int64 // absolute, -1 if unknown
	tracksPos    int64 // absolute, -1 if unknown
	seekHeads    []int64
	firstCluster int64 // absolute, -1 if not seen
	haveInfo     bool
	haveTracks   bool
}

func (rd *reader) index() (*Index, error) {
	head, err := rd.readAt(0, headRead)
	if err != nil {
		return nil, err
	}
	id, sz, hl, err := element(head, 0)
	if err != nil || id != idEBML || sz < 0 {
		return nil, ErrNotMatroska
	}
	pos := int64(hl) + sz
	id, sz, hl, err = element(head, int(pos))
	if err != nil || id != idSegment {
		return nil, ErrNotMatroska
	}
	m := &segmentMeta{start: pos + int64(hl), scale: 1_000_000, cuesPos: -1, infoPos: -1, tracksPos: -1, firstCluster: -1}
	m.end = rd.size
	if sz >= 0 {
		m.end = min(rd.size, m.start+sz)
	}

	var cues []byte
	if err := rd.topLevel(head, m, &cues); err != nil {
		return nil, err
	}
	// SeekHeads (the first one may point to another at the end of the file).
	for i := 0; i < len(m.seekHeads) && i < 4 && (m.cuesPos < 0 || !m.haveInfo || !m.haveTracks); i++ {
		if err := rd.readSeekHeadAt(m, m.seekHeads[i]); err != nil {
			return nil, err
		}
	}
	if !m.haveInfo && m.infoPos >= 0 {
		if b, err := rd.readElement(m.infoPos, idInfo, maxMetaRead); err == nil {
			m.parseInfo(b)
		}
	}
	if !m.haveTracks && m.tracksPos >= 0 {
		if b, err := rd.readElement(m.tracksPos, idTracks, maxMetaRead); err == nil {
			m.parseTracks(b)
		}
	}
	if !m.haveTracks || m.videoTrack == 0 {
		return nil, fmt.Errorf("%w: no video track", ErrNoCues)
	}
	if cues == nil {
		if m.cuesPos < 0 {
			return nil, ErrNoCues
		}
		cues, err = rd.readElement(m.cuesPos, idCues, maxCuesRead)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrNoCues, err)
		}
	}
	ticks, clusters, err := parseCues(cues, m.videoTrack)
	if err != nil {
		return nil, err
	}
	if len(ticks) == 0 {
		return nil, fmt.Errorf("%w: no cue point for the video track", ErrNoCues)
	}
	if err := rd.verify(m, ticks, clusters); err != nil {
		return nil, err
	}
	times := make([]float64, len(ticks))
	for i, t := range ticks {
		times[i] = float64(t) * float64(m.scale) / 1e9
	}
	return &Index{Times: times}, nil
}

// topLevel walks the top-level elements from the start of the segment up to
// the first cluster, reading SeekHead, Info, Tracks (and Cues placed in
// front, as `mkvmerge --cues` front placement does).
func (rd *reader) topLevel(head []byte, m *segmentMeta, cues *[]byte) error {
	pos := m.start
	for n := 0; n < maxTopLevel && pos < m.end; n++ {
		hdr, err := rd.bytesAt(head, pos, 16)
		if err != nil {
			return err
		}
		id, sz, hl, err := element(hdr, 0)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrNoCues, err)
		}
		if id == idCluster {
			m.firstCluster = pos
			return nil
		}
		if sz < 0 {
			return nil // unknown-size element other than a cluster: give up walking
		}
		body := pos + int64(hl)
		switch id {
		case idSeekHead, idInfo, idTracks:
			if sz > maxMetaRead {
				return fmt.Errorf("%w: element too large", ErrNoCues)
			}
			b, err := rd.bytesAt(head, body, sz)
			if err != nil {
				return err
			}
			switch id {
			case idSeekHead:
				m.parseSeekHead(b)
			case idInfo:
				m.parseInfo(b)
			case idTracks:
				m.parseTracks(b)
			}
		case idCues:
			if sz <= maxCuesRead {
				b, err := rd.bytesAt(head, body, sz)
				if err != nil {
					return err
				}
				*cues = b
			}
		}
		pos = body + sz
	}
	return nil
}

// bytesAt returns n bytes at off, from head when it holds them.
func (rd *reader) bytesAt(head []byte, off, n int64) ([]byte, error) {
	if off >= 0 && off+n <= int64(len(head)) {
		return head[off : off+n], nil
	}
	if off < int64(len(head)) && off+n > int64(len(head)) && int64(len(head)) == rd.size {
		return head[off:], nil // short file
	}
	return rd.readAt(off, n)
}

// readElement reads the payload of the element of the given id at off.
func (rd *reader) readElement(off int64, want uint64, limit int64) ([]byte, error) {
	hdr, err := rd.readAt(off, 16)
	if err != nil {
		return nil, err
	}
	id, sz, hl, err := element(hdr, 0)
	if err != nil {
		return nil, err
	}
	if id != want {
		return nil, fmt.Errorf("mkv: expected element %#x at %d, found %#x", want, off, id)
	}
	if sz < 0 || sz > limit {
		return nil, fmt.Errorf("mkv: element %#x has an unusable size (%d)", want, sz)
	}
	return rd.readAt(off+int64(hl), sz)
}

func (rd *reader) readSeekHeadAt(m *segmentMeta, off int64) error {
	b, err := rd.readElement(off, idSeekHead, maxMetaRead)
	if err != nil {
		return nil // a broken secondary SeekHead is not fatal: the Cues may be known already
	}
	m.parseSeekHead(b)
	return nil
}

func (m *segmentMeta) parseSeekHead(b []byte) {
	_ = children(b, func(id uint64, payload []byte) {
		if id != idSeek {
			return
		}
		var target uint64
		var at int64 = -1
		_ = children(payload, func(id uint64, v []byte) {
			switch id {
			case idSeekID:
				target = rawUint(v)
			case idSeekPosition:
				at = int64(rawUint(v))
			}
		})
		if at < 0 {
			return
		}
		abs := m.start + at
		switch target {
		case idCues:
			if m.cuesPos < 0 {
				m.cuesPos = abs
			}
		case idInfo:
			if m.infoPos < 0 {
				m.infoPos = abs
			}
		case idTracks:
			if m.tracksPos < 0 {
				m.tracksPos = abs
			}
		case idSeekHead:
			for _, s := range m.seekHeads {
				if s == abs {
					return
				}
			}
			m.seekHeads = append(m.seekHeads, abs)
		}
	})
}

func (m *segmentMeta) parseInfo(b []byte) {
	m.haveInfo = true
	_ = children(b, func(id uint64, v []byte) {
		if id == idTimecodeScale {
			if s := rawUint(v); s > 0 {
				m.scale = s
			}
		}
	})
}

func (m *segmentMeta) parseTracks(b []byte) {
	m.haveTracks = true
	_ = children(b, func(id uint64, entry []byte) {
		if id != idTrackEntry || m.videoTrack != 0 {
			return
		}
		var num, typ uint64
		_ = children(entry, func(id uint64, v []byte) {
			switch id {
			case idTrackNumber:
				num = rawUint(v)
			case idTrackType:
				typ = rawUint(v)
			}
		})
		if typ == trackTypeVideo && num > 0 {
			m.videoTrack = num
		}
	})
}

// parseCues returns the sorted, unique cue times (ticks) of track and the
// cluster positions (relative to the segment) they point to, by time.
func parseCues(b []byte, track uint64) ([]uint64, map[uint64]int64, error) {
	var ticks []uint64
	clusters := map[uint64]int64{}
	err := children(b, func(id uint64, cp []byte) {
		if id != idCuePoint {
			return
		}
		var t uint64
		haveTime, match := false, false
		var cluster int64 = -1
		_ = children(cp, func(id uint64, v []byte) {
			switch id {
			case idCueTime:
				t, haveTime = rawUint(v), true
			case idCueTrackPos:
				var tr uint64
				var cl int64 = -1
				_ = children(v, func(id uint64, w []byte) {
					switch id {
					case idCueTrack:
						tr = rawUint(w)
					case idCueClusterPos:
						cl = int64(rawUint(w))
					}
				})
				if tr == track {
					match = true
					cluster = cl
				}
			}
		})
		if haveTime && match {
			ticks = append(ticks, t)
			if cluster >= 0 {
				clusters[t] = cluster
			}
		}
	})
	if err != nil && len(ticks) == 0 {
		return nil, nil, fmt.Errorf("%w: %w", ErrNoCues, err)
	}
	sort.Slice(ticks, func(i, j int) bool { return ticks[i] < ticks[j] })
	out := ticks[:0]
	for i, t := range ticks {
		if i == 0 || t != out[len(out)-1] {
			out = append(out, t)
		}
	}
	return out, clusters, nil
}

// verify reads the start of a few clusters (the first one, one in the
// middle, one near the end) and checks that every video keyframe found
// there is listed in the index.
func (rd *reader) verify(m *segmentMeta, ticks []uint64, clusters map[uint64]int64) error {
	set := make(map[uint64]struct{}, len(ticks))
	for _, t := range ticks {
		set[t] = struct{}{}
	}
	var samples []int64
	if m.firstCluster >= 0 {
		samples = append(samples, m.firstCluster)
	}
	for _, i := range []int{len(ticks) / 2, len(ticks) * 9 / 10} {
		if p, ok := clusters[ticks[i]]; ok {
			abs := m.start + p
			if len(samples) == 0 || samples[len(samples)-1] != abs {
				samples = append(samples, abs)
			}
		}
	}
	if len(samples) == 0 {
		return fmt.Errorf("%w: no cluster to check the index against", ErrNoCues)
	}
	checked := 0
	for _, off := range samples {
		if off >= m.end {
			continue
		}
		buf, err := rd.readAt(off, sampleWindow)
		if err != nil {
			return err
		}
		keys, err := clusterKeyframes(buf, m.videoTrack)
		if err != nil {
			return fmt.Errorf("%w: cluster at %d: %w", ErrNoCues, off, err)
		}
		for _, k := range keys {
			checked++
			if _, ok := set[k]; !ok {
				return fmt.Errorf("%w: keyframe at %.3fs missing", ErrIncompleteCues, float64(k)*float64(m.scale)/1e9)
			}
		}
	}
	if checked == 0 {
		return fmt.Errorf("%w: no keyframe found in the sampled clusters", ErrNoCues)
	}
	return nil
}

// clusterKeyframes lists the timestamps (ticks) of the video keyframes
// found in buf, which starts with a Cluster element and may be truncated.
// Clusters following the first one inside buf are parsed too.
func clusterKeyframes(buf []byte, track uint64) ([]uint64, error) {
	var keys []uint64
	pos := 0
	for pos < len(buf) {
		id, sz, hl, err := element(buf, pos)
		if err != nil {
			break // truncated header at the end of the window
		}
		if id != idCluster {
			if len(keys) == 0 && pos == 0 {
				return nil, fmt.Errorf("expected a cluster, found %#x", id)
			}
			break
		}
		body := pos + hl
		end := len(buf)
		if sz >= 0 && body+int(sz) < end {
			end = body + int(sz)
		}
		var clusterTime uint64
		haveTime := false
		p := body
		for p < end {
			cid, csz, chl, err := element(buf[:end], p)
			if err != nil {
				break
			}
			if cid == idCluster { // next cluster of an unknown-size cluster
				end = p
				break
			}
			if csz < 0 {
				return keys, nil
			}
			payloadEnd := p + chl + int(csz)
			if payloadEnd > end {
				// Truncated: a block header may still be readable.
				if cid == idSimpleBlock && haveTime {
					if k, ok := blockKey(buf[p+chl:end], track, true, clusterTime); ok {
						keys = append(keys, k)
					}
				}
				return keys, nil
			}
			payload := buf[p+chl : payloadEnd]
			switch cid {
			case idClusterTime:
				clusterTime, haveTime = rawUint(payload), true
			case idSimpleBlock:
				if haveTime {
					if k, ok := blockKey(payload, track, true, clusterTime); ok {
						keys = append(keys, k)
					}
				}
			case idBlockGroup:
				if haveTime {
					var block []byte
					ref := false
					_ = children(payload, func(id uint64, v []byte) {
						switch id {
						case idBlock:
							block = v
						case idReferenceBlock:
							ref = true
						}
					})
					if block != nil && !ref {
						if k, ok := blockKey(block, track, false, clusterTime); ok {
							keys = append(keys, k)
						}
					}
				}
			}
			p = payloadEnd
		}
		pos = end
	}
	return keys, nil
}

// blockKey decodes a (Simple)Block header: track number, signed 16-bit
// relative timestamp, flags. simple: the keyframe flag is meaningful.
func blockKey(b []byte, track uint64, simple bool, clusterTime uint64) (uint64, bool) {
	tn, n, err := vint(b, 0, false)
	if err != nil || len(b) < n+3 || tn != track {
		return 0, false
	}
	rel := int64(int16(binary.BigEndian.Uint16(b[n : n+2])))
	if simple && b[n+2]&0x80 == 0 {
		return 0, false
	}
	t := int64(clusterTime) + rel
	if t < 0 {
		return 0, false
	}
	return uint64(t), true
}

// children calls fn for each child element contained in b.
func children(b []byte, fn func(id uint64, payload []byte)) error {
	pos := 0
	for pos < len(b) {
		id, sz, hl, err := element(b, pos)
		if err != nil {
			return err
		}
		if sz < 0 || pos+hl+int(sz) > len(b) {
			return errors.New("mkv: truncated element")
		}
		fn(id, b[pos+hl:pos+hl+int(sz)])
		pos += hl + int(sz)
	}
	return nil
}

// element decodes the element header at b[pos:]: id (with marker), payload
// size (-1 when unknown) and header length.
func element(b []byte, pos int) (id uint64, size int64, hdrLen int, err error) {
	id, n, err := vint(b, pos, true)
	if err != nil {
		return 0, 0, 0, err
	}
	if n > 4 {
		return 0, 0, 0, errors.New("mkv: invalid element id")
	}
	s, m, err := vint(b, pos+n, false)
	if err != nil {
		return 0, 0, 0, err
	}
	size = int64(s)
	if s == (uint64(1)<<(7*m))-1 { // all value bits set: unknown size
		size = -1
	} else if s > math.MaxInt64/2 {
		return 0, 0, 0, errors.New("mkv: element size overflow")
	}
	return id, size, n + m, nil
}

// vint decodes an EBML variable-length integer at b[pos:]. keepMarker keeps
// the length marker bit (element IDs).
func vint(b []byte, pos int, keepMarker bool) (uint64, int, error) {
	if pos >= len(b) || pos < 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	first := b[pos]
	if first == 0 {
		return 0, 0, errors.New("mkv: invalid vint")
	}
	n := 1
	for mask := byte(0x80); first&mask == 0; mask >>= 1 {
		n++
	}
	if pos+n > len(b) {
		return 0, 0, io.ErrUnexpectedEOF
	}
	v := uint64(first)
	if !keepMarker {
		v &= uint64(0xFF >> n)
	}
	for i := 1; i < n; i++ {
		v = v<<8 | uint64(b[pos+i])
	}
	return v, n, nil
}

// rawUint decodes a big-endian unsigned integer payload (0 to 8 bytes).
func rawUint(b []byte) uint64 {
	var v uint64
	for _, c := range b[:min(len(b), 8)] {
		v = v<<8 | uint64(c)
	}
	return v
}
