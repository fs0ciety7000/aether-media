package probe

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// flexValue decodes a JSON string or number; ffprobe emits most numeric
// fields as strings ("1.023000", "128000") and some as numbers.
type flexValue string

func (f *flexValue) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexValue(s)
		return nil
	}
	*f = flexValue(b)
	return nil
}

func (f flexValue) int64() int64 {
	s := strings.TrimSpace(string(f))
	if s == "" || strings.EqualFold(s, "N/A") {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if x, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(x) && !math.IsInf(x, 0) {
		return int64(x)
	}
	return 0
}

func (f flexValue) float() float64 {
	s := strings.TrimSpace(string(f))
	if s == "" || strings.EqualFold(s, "N/A") {
		return 0
	}
	x, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return x
}

type rawOutput struct {
	Streams  []rawStream  `json:"streams"`
	Chapters []rawChapter `json:"chapters"`
	Format   *rawFormat   `json:"format"`
	Error    *struct {
		Code   int    `json:"code"`
		String string `json:"string"`
	} `json:"error"`
}

type rawFormat struct {
	Filename   string            `json:"filename"`
	NbStreams  int               `json:"nb_streams"`
	FormatName string            `json:"format_name"`
	Duration   flexValue         `json:"duration"`
	Size       flexValue         `json:"size"`
	BitRate    flexValue         `json:"bit_rate"`
	Tags       map[string]string `json:"tags"`
}

type rawStream struct {
	Index             int               `json:"index"`
	CodecName         string            `json:"codec_name"`
	CodecType         string            `json:"codec_type"`
	CodecTagString    string            `json:"codec_tag_string"`
	Profile           flexValue         `json:"profile"`
	Level             flexValue         `json:"level"`
	Width             int               `json:"width"`
	Height            int               `json:"height"`
	PixFmt            string            `json:"pix_fmt"`
	FieldOrder        string            `json:"field_order"`
	ColorTransfer     string            `json:"color_transfer"`
	ColorPrimaries    string            `json:"color_primaries"`
	ColorSpace        string            `json:"color_space"`
	RFrameRate        string            `json:"r_frame_rate"`
	AvgFrameRate      string            `json:"avg_frame_rate"`
	BitsPerRawSample  flexValue         `json:"bits_per_raw_sample"`
	BitsPerSample     flexValue         `json:"bits_per_sample"`
	SampleFmt         string            `json:"sample_fmt"`
	SampleRate        flexValue         `json:"sample_rate"`
	Channels          int               `json:"channels"`
	ChannelLayout     string            `json:"channel_layout"`
	BitRate           flexValue         `json:"bit_rate"`
	Duration          flexValue         `json:"duration"`
	Disposition       map[string]int    `json:"disposition"`
	Tags              map[string]string `json:"tags"`
	SideDataList      []rawSideData     `json:"side_data_list"`
	NbFrames          flexValue         `json:"nb_frames"`
	ExtradataSize     int               `json:"extradata_size"`
	ClosedCaptions    int               `json:"closed_captions"`
	HasBFrames        int               `json:"has_b_frames"`
	SampleAspectRatio string            `json:"sample_aspect_ratio"`
}

type rawSideData struct {
	SideDataType            string `json:"side_data_type"`
	DvProfile               int    `json:"dv_profile"`
	DvLevel                 int    `json:"dv_level"`
	DvBlSignalCompatibility int    `json:"dv_bl_signal_compatibility_id"`
}

type rawChapter struct {
	StartTime flexValue         `json:"start_time"`
	EndTime   flexValue         `json:"end_time"`
	Tags      map[string]string `json:"tags"`
}

// ParseOutput parses ffprobe JSON output (-show_format -show_streams
// -show_chapters) and normalises it. path is used to refine the container
// name from the file extension; it may be empty.
func ParseOutput(data []byte, path string) (*Info, error) {
	var raw rawOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}
	if raw.Error != nil && raw.Error.String != "" {
		if isNotMediaMessage(raw.Error.String) {
			return nil, fmt.Errorf("%s: %w", raw.Error.String, ErrNotMedia)
		}
		return nil, errors.New("ffprobe: " + raw.Error.String)
	}
	if raw.Format == nil && len(raw.Streams) == 0 {
		return nil, fmt.Errorf("no format or streams in ffprobe output: %w", ErrNotMedia)
	}
	if len(raw.Streams) == 0 {
		return nil, fmt.Errorf("no streams: %w", ErrNotMedia)
	}
	if path == "" && raw.Format != nil {
		path = raw.Format.Filename
	}

	info := &Info{Tags: map[string]string{}}
	var formatName string
	if f := raw.Format; f != nil {
		formatName = f.FormatName
		info.DurationMs = secondsToMs(f.Duration.float())
		info.Size = f.Size.int64()
		info.Bitrate = f.BitRate.int64()
		info.Tags = normaliseFormatTags(f.Tags)
	}
	info.Container = NormaliseContainer(formatName, path, info.Tags["major_brand"])

	audioContainer := isAudioContainer(info.Container)
	for _, rs := range raw.Streams {
		info.Streams = append(info.Streams, normaliseStream(rs, audioContainer))
	}

	// Ogg/Opus files carry their metadata on the audio stream, not the format.
	if info.Container == "ogg" || info.Container == "opus" {
		for _, rs := range raw.Streams {
			if rs.CodecType == TypeAudio {
				for k, v := range normaliseFormatTags(rs.Tags) {
					if _, ok := info.Tags[k]; !ok && !isTechnicalTag(k) {
						info.Tags[k] = v
					}
				}
				break
			}
		}
	}

	if info.DurationMs == 0 {
		info.DurationMs = longestStreamMs(raw.Streams)
	}
	if info.Bitrate == 0 && info.Size > 0 && info.DurationMs > 0 {
		info.Bitrate = info.Size * 8 * 1000 / info.DurationMs
	}
	info.Chapters = normaliseChapters(raw.Chapters, info.DurationMs)
	return info, nil
}

func secondsToMs(sec float64) int64 {
	if sec <= 0 {
		return 0
	}
	return int64(math.Round(sec * 1000))
}

func longestStreamMs(streams []rawStream) int64 {
	var best int64
	for _, s := range streams {
		ms := secondsToMs(s.Duration.float())
		if ms == 0 {
			ms = parseClockMs(tagValue(s.Tags, "duration"))
		}
		if ms > best {
			best = ms
		}
	}
	return best
}

// parseClockMs parses Matroska "DURATION" statistics tags ("00:01:02.500000000").
func parseClockMs(v string) int64 {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 3 {
		return 0
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	s, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil || h < 0 || m < 0 || s < 0 {
		return 0
	}
	return int64(h)*3_600_000 + int64(m)*60_000 + int64(math.Round(s*1000))
}

// ParseRational parses ffprobe rationals ("24000/1001", "25/1", "0/0", "25").
// Invalid, zero-denominator or negative values yield 0.
func ParseRational(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	num, den, hasDen := strings.Cut(v, "/")
	if !hasDen {
		num, den, hasDen = strings.Cut(v, ":")
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	if !hasDen {
		if n < 0 {
			return 0
		}
		return n
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(den), 64)
	if err != nil || d == 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		return 0
	}
	r := n / d
	if r < 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		return 0
	}
	return r
}

// tagValue looks up a tag case-insensitively.
func tagValue(tags map[string]string, key string) string {
	if v, ok := tags[key]; ok {
		return strings.TrimSpace(v)
	}
	for k, v := range tags {
		if strings.EqualFold(k, key) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

var imageSubCodecs = map[string]bool{
	"hdmv_pgs_subtitle": true,
	"pgssub":            true,
	"dvd_subtitle":      true,
	"dvdsub":            true,
	"dvb_subtitle":      true,
	"dvbsub":            true,
	"xsub":              true,
	"vobsub":            true,
}

// IsImageSubtitleCodec reports whether a subtitle codec is bitmap-based.
func IsImageSubtitleCodec(codec string) bool {
	return imageSubCodecs[strings.ToLower(codec)]
}

// Still-image codecs that, as a "video" stream, are always cover art.
var stillImageCodecs = map[string]bool{"png": true, "bmp": true, "gif": true, "webp": true, "jpeg": true}

func normaliseStream(rs rawStream, audioContainer bool) Stream {
	s := Stream{
		Index:         rs.Index,
		Type:          strings.ToLower(rs.CodecType),
		Codec:         strings.ToLower(rs.CodecName),
		Profile:       strings.TrimSpace(string(rs.Profile)),
		Width:         rs.Width,
		Height:        rs.Height,
		PixFmt:        rs.PixFmt,
		Channels:      rs.Channels,
		ChannelLayout: rs.ChannelLayout,
		SampleRate:    int(rs.SampleRate.int64()),
		Language:      NormaliseLanguage(tagValue(rs.Tags, "language")),
		Title:         tagValue(rs.Tags, "title"),
		Default:       rs.Disposition["default"] == 1,
		Forced:        rs.Disposition["forced"] == 1,
		Bitrate:       rs.BitRate.int64(),
	}
	if s.Type == "" {
		s.Type = TypeData
	}
	if s.Profile == "unknown" || s.Profile == "-99" {
		s.Profile = ""
	}
	if lvl := rs.Level.int64(); lvl > 0 {
		s.Level = int(lvl)
	}
	s.HearingImpaired = rs.Disposition["hearing_impaired"] == 1
	if s.Bitrate == 0 {
		// Matroska statistics tags written by mkvmerge.
		for _, k := range []string{"bps", "bps-eng"} {
			if b := parseInt64(tagValue(rs.Tags, k)); b > 0 {
				s.Bitrate = b
				break
			}
		}
	}

	switch s.Type {
	case TypeVideo:
		s.IsAttachedPic = rs.Disposition["attached_pic"] == 1 ||
			stillImageCodecs[s.Codec] ||
			(audioContainer && s.Codec == "mjpeg")
		s.FrameRate = ParseRational(rs.AvgFrameRate)
		if s.FrameRate == 0 {
			s.FrameRate = ParseRational(rs.RFrameRate)
		}
		if s.IsAttachedPic {
			s.FrameRate = 0
		}
		fo := strings.ToLower(rs.FieldOrder)
		s.Interlaced = fo != "" && fo != "progressive" && fo != "unknown"
		s.BitDepth = int(rs.BitsPerRawSample.int64())
		if s.BitDepth == 0 {
			s.BitDepth = bitDepthFromPixFmt(rs.PixFmt)
		}
		s.ColorTransfer = rs.ColorTransfer
		s.ColorPrimaries = rs.ColorPrimaries
		s.ColorSpace = rs.ColorSpace
		if !s.IsAttachedPic {
			detectHDR(&s, rs)
		}
	case TypeAudio:
		s.BitDepth = int(rs.BitsPerRawSample.int64())
		if s.BitDepth == 0 {
			s.BitDepth = int(rs.BitsPerSample.int64())
		}
		if s.ChannelLayout == "" && s.Channels > 0 {
			s.ChannelLayout = defaultLayout(s.Channels)
		}
	case TypeSubtitle:
		s.IsImageSub = IsImageSubtitleCodec(s.Codec)
		t := strings.ToLower(s.Title)
		if !s.Forced && containsWord(t, "forced") {
			s.Forced = true
		}
		if !s.HearingImpaired && (containsWord(t, "sdh") || containsWord(t, "cc") ||
			strings.Contains(t, "hearing impaired")) {
			s.HearingImpaired = true
		}
	case TypeAttachment:
		s.Filename = tagValue(rs.Tags, "filename")
		s.MimeType = tagValue(rs.Tags, "mimetype")
	}
	return s
}

func parseInt64(v string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// containsWord reports whether word appears in s delimited by non-letters.
func containsWord(s, word string) bool {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, f := range fields {
		if f == word {
			return true
		}
	}
	return false
}

func detectHDR(s *Stream, rs rawStream) {
	hdr10Plus := false
	for _, sd := range rs.SideDataList {
		t := strings.ToLower(sd.SideDataType)
		switch {
		case strings.Contains(t, "dovi configuration record"):
			s.HDR = DOVI
			s.DoviProfile = sd.DvProfile
			s.DoviLevel = sd.DvLevel
			s.DoviCompatID = sd.DvBlSignalCompatibility
		case strings.Contains(t, "smpte2094-40") || strings.Contains(t, "hdr10+"):
			hdr10Plus = true
		}
	}
	if s.HDR == DOVI {
		return
	}
	switch strings.ToLower(rs.CodecTagString) {
	case "dvh1", "dvhe", "dav1", "dva1", "dvav":
		s.HDR = DOVI
		return
	}
	switch strings.ToLower(rs.ColorTransfer) {
	case "smpte2084":
		if hdr10Plus {
			s.HDR = HDR10Plus
		} else {
			s.HDR = HDR10
		}
	case "arib-std-b67":
		s.HDR = HLG
	}
}

// bitDepthFromPixFmt infers the per-component bit depth from a pixel format
// name: yuv420p10le → 10, p010le → 10, gray12be → 12, yuv420p/nv12/rgb24 → 8.
func bitDepthFromPixFmt(pf string) int {
	pf = strings.ToLower(strings.TrimSpace(pf))
	if pf == "" {
		return 0
	}
	pf = strings.TrimSuffix(strings.TrimSuffix(pf, "le"), "be")
	if len(pf) == 4 && pf[0] == 'p' && pf[1] >= '0' && pf[1] <= '4' { // p010, p016, p210, p410
		if n, err := strconv.Atoi(pf[2:]); err == nil && n >= 8 && n <= 16 {
			return n
		}
	}
	i := len(pf)
	for i > 0 && pf[i-1] >= '0' && pf[i-1] <= '9' {
		i--
	}
	if i < len(pf) && i > 0 && (pf[i-1] == 'p' || pf[i-1] == 'y') {
		if n, err := strconv.Atoi(pf[i:]); err == nil && n >= 8 && n <= 16 {
			return n
		}
	}
	return 8
}

func defaultLayout(ch int) string {
	switch ch {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	}
	return ""
}

func normaliseChapters(raw []rawChapter, durationMs int64) []Chapter {
	if len(raw) == 0 {
		return nil
	}
	out := make([]Chapter, 0, len(raw))
	for _, c := range raw {
		ch := Chapter{
			StartMs: secondsToMs(c.StartTime.float()),
			EndMs:   secondsToMs(c.EndTime.float()),
			Title:   tagValue(c.Tags, "title"),
		}
		if durationMs > 0 && ch.EndMs > durationMs {
			ch.EndMs = durationMs
		}
		if ch.EndMs < ch.StartMs {
			ch.EndMs = ch.StartMs
		}
		out = append(out, ch)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].StartMs < out[b].StartMs })
	return out
}

// Canonical names for common tag aliases (keys already lower-cased).
var tagAliases = map[string]string{
	"tracknumber":  "track",
	"trck":         "track",
	"discnumber":   "disc",
	"disk":         "disc",
	"tpos":         "disc",
	"albumartist":  "album_artist",
	"album artist": "album_artist",
	"tpe2":         "album_artist",
	"year":         "date",
	"tyer":         "date",
	"tdrc":         "date",
	"originaldate": "original_date",
	"tcon":         "genre",
	"tit2":         "title",
	"tpe1":         "artist",
	"talb":         "album",
	"description":  "description",
	"synopsis":     "synopsis",
	"totaltracks":  "tracktotal",
	"totaldiscs":   "disctotal",
}

func normaliseFormatTags(tags map[string]string) map[string]string {
	out := make(map[string]string, len(tags))
	// Insert exact lower-case keys first so they win over aliases.
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := strings.TrimSpace(tags[k])
		if v == "" {
			continue
		}
		lk := strings.ToLower(strings.TrimSpace(k))
		if _, ok := out[lk]; !ok {
			out[lk] = v
		}
	}
	for alias, canon := range tagAliases {
		if v, ok := out[alias]; ok {
			if _, exists := out[canon]; !exists {
				out[canon] = v
			}
		}
	}
	return out
}

func isTechnicalTag(k string) bool {
	switch k {
	case "encoder", "language", "duration", "handler_name", "vendor_id", "bps", "bps-eng",
		"number_of_frames", "number_of_bytes", "_statistics_writing_app", "_statistics_tags",
		"_statistics_writing_date_utc":
		return true
	}
	return false
}

// NormaliseContainer maps an ffprobe format_name (possibly a comma-separated
// demuxer list) to a short container name, using the file extension and the
// MP4 major brand to disambiguate shared demuxers.
func NormaliseContainer(formatName, path, majorBrand string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if i := strings.IndexAny(ext, "?#"); i >= 0 { // URLs
		ext = ext[:i]
	}
	f := strings.ToLower(strings.TrimSpace(formatName))
	switch f {
	case "":
		return ext
	case "matroska,webm", "matroska", "webm":
		if ext == "webm" || f == "webm" {
			return "webm"
		}
		return "mkv"
	case "mov,mp4,m4a,3gp,3g2,mj2":
		switch ext {
		case "m4a", "m4b", "m4r":
			return "m4a"
		case "m4v":
			return "m4v"
		case "mov", "qt":
			return "mov"
		case "3gp", "3g2":
			return ext
		case "mp4":
			return "mp4"
		}
		if strings.TrimSpace(majorBrand) == "qt" {
			return "mov"
		}
		if strings.HasPrefix(strings.TrimSpace(strings.ToLower(majorBrand)), "m4a") {
			return "m4a"
		}
		return "mp4"
	case "mpegts":
		return "ts"
	case "mpeg", "mpegvideo":
		if ext == "vob" {
			return "vob"
		}
		return "mpeg"
	case "asf":
		if ext == "wmv" || ext == "wma" {
			return ext
		}
		return "asf"
	case "ogg":
		if ext == "opus" {
			return "opus"
		}
		return "ogg"
	case "webvtt":
		return "vtt"
	case "aiff":
		return "aiff"
	}
	if strings.HasSuffix(f, "_pipe") {
		switch img := strings.TrimSuffix(f, "_pipe"); img {
		case "jpeg":
			return "jpg"
		default:
			return img
		}
	}
	first, _, _ := strings.Cut(f, ",")
	return first
}

var audioContainers = map[string]bool{
	"mp3": true, "flac": true, "m4a": true, "ogg": true, "opus": true, "wav": true,
	"aiff": true, "ape": true, "wv": true, "wma": true, "dsf": true, "tak": true,
}

func isAudioContainer(c string) bool { return audioContainers[c] }
