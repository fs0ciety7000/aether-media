package probe

import "strings"

// Stream types.
const (
	TypeVideo      = "video"
	TypeAudio      = "audio"
	TypeSubtitle   = "subtitle"
	TypeAttachment = "attachment"
	TypeData       = "data"
)

// HDR formats reported in Stream.HDR.
const (
	HDR10     = "HDR10"
	HDR10Plus = "HDR10+"
	HLG       = "HLG"
	DOVI      = "DOVI"
)

// Info is the normalised result of probing one file.
type Info struct {
	Container  string // normalised: "mkv", "webm", "mp4", "m4a", "mov", "ts", "avi", "mp3", "flac"...
	DurationMs int64
	Bitrate    int64 // bits/s
	Size       int64
	Streams    []Stream
	Chapters   []Chapter
	Tags       map[string]string // format tags, lower-cased keys (title, artist, album_artist, album, track, disc, date, genre, ...)
}

// Stream describes one elementary stream.
type Stream struct {
	Index           int
	Type            string // "video" | "audio" | "subtitle" | "attachment" | "data"
	Codec           string // ffprobe codec_name
	Profile         string // "Main 10", "DTS-HD MA", "LC"...
	Level           int    // raw ffprobe level (h264: 41, hevc: 150 = 5.0 × 30); 0 when unknown
	Width, Height   int
	FrameRate       float64 // avg_frame_rate, falling back to r_frame_rate
	BitDepth        int
	PixFmt          string
	Interlaced      bool
	HDR             string // "" | "HDR10" | "HDR10+" | "HLG" | "DOVI"
	DoviProfile     int    // Dolby Vision profile (5, 7, 8...) when HDR == "DOVI"
	DoviLevel       int    // Dolby Vision level
	DoviCompatID    int    // Dolby Vision base layer signal compatibility id (1 = HDR10, 2 = SDR, 4 = HLG)
	ColorTransfer   string // "smpte2084", "arib-std-b67", "bt709"...
	ColorPrimaries  string // "bt2020", "bt709"...
	ColorSpace      string // "bt2020nc", "bt709"...
	Channels        int
	ChannelLayout   string
	SampleRate      int
	Language        string // ISO 639-2/B lowercase ("fre", "eng"); "" when undetermined
	Title           string
	Default         bool
	Forced          bool
	HearingImpaired bool
	Bitrate         int64
	IsImageSub      bool   // bitmap subtitle (PGS, VobSub, DVB, XSUB)
	IsAttachedPic   bool   // embedded cover art
	Filename        string // attachment file name (fonts in mkv), from tags
	MimeType        string // attachment mime type, from tags
}

// Chapter is one chapter marker.
type Chapter struct {
	StartMs, EndMs int64
	Title          string
}

// Video returns the first real video stream (cover art excluded), or nil.
func (i *Info) Video() *Stream {
	if i == nil {
		return nil
	}
	for k := range i.Streams {
		s := &i.Streams[k]
		if s.Type == TypeVideo && !s.IsAttachedPic {
			return s
		}
	}
	return nil
}

// AudioStreams returns the audio streams in file order.
func (i *Info) AudioStreams() []Stream { return i.streamsOf(TypeAudio) }

// SubtitleStreams returns the subtitle streams in file order.
func (i *Info) SubtitleStreams() []Stream { return i.streamsOf(TypeSubtitle) }

// AttachedPic returns the first embedded cover-art stream, or nil.
func (i *Info) AttachedPic() *Stream {
	if i == nil {
		return nil
	}
	for k := range i.Streams {
		if i.Streams[k].IsAttachedPic {
			return &i.Streams[k]
		}
	}
	return nil
}

// IsAudioOnly reports whether the file has audio but no real video stream.
func (i *Info) IsAudioOnly() bool {
	return i.Video() == nil && len(i.AudioStreams()) > 0
}

func (i *Info) streamsOf(typ string) []Stream {
	if i == nil {
		return nil
	}
	var out []Stream
	for _, s := range i.Streams {
		if s.Type == typ {
			out = append(out, s)
		}
	}
	return out
}

// IsAtmos reports whether the audio stream carries Dolby Atmos (TrueHD or E-AC-3 JOC).
func (s *Stream) IsAtmos() bool {
	return s.Type == TypeAudio && strings.Contains(strings.ToLower(s.Profile), "atmos")
}

// IsTextSub reports whether the stream is a text subtitle (convertible to WebVTT).
func (s *Stream) IsTextSub() bool {
	return s.Type == TypeSubtitle && !s.IsImageSub
}
