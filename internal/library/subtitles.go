package library

import (
	"context"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// externalSubIndexBase keeps side-car subtitle indexes clear of ffprobe's.
const externalSubIndexBase = 1000

var subtitleCodecs = map[string]string{".srt": "subrip", ".ass": "ass", ".ssa": "ass", ".vtt": "webvtt", ".sub": "microdvd"}

// sidecar is an external subtitle file next to a video.
type sidecar struct {
	path, codec, language, title string
	forced, hearingImpaired      bool
}

// findSidecars lists subtitle files named after the video:
// "Movie.srt", "Movie.fr.srt", "Movie.fre.forced.srt", "Movie.en.sdh.srt"...
// entries is the listing of the video's directory.
func findSidecars(videoPath string, entries []vfs.Entry) []sidecar {
	dir := filepath.Dir(videoPath)
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath)))
	var out []sidecar
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		name := path.Base(e.Path)
		ext := strings.ToLower(filepath.Ext(name))
		codec, ok := subtitleCodecs[ext]
		if !ok {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		if stem != base && !strings.HasPrefix(stem, base+".") {
			continue
		}
		s := sidecar{path: filepath.Join(dir, name), codec: codec}
		var titleParts []string
		for _, tok := range strings.Split(strings.TrimPrefix(stem, base), ".") {
			switch tok {
			case "":
			case "forced", "force", "foreign":
				s.forced = true
			case "sdh", "cc", "hi":
				s.hearingImpaired = true
			default:
				if lang := probe.NormaliseLanguage(tok); lang != "" && s.language == "" {
					s.language = lang
				} else {
					titleParts = append(titleParts, tok)
				}
			}
		}
		s.title = strings.Join(titleParts, " ")
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// saveSidecars replaces a media file's external subtitle streams.
func saveSidecars(ctx context.Context, q *db.Queries, mediaFileID int64, subs []sidecar) error {
	if err := q.DeleteExternalStreams(ctx, mediaFileID); err != nil {
		return err
	}
	for i, s := range subs {
		err := q.InsertExternalStream(ctx, db.InsertExternalStreamParams{
			MediaFileID: mediaFileID, Idx: int64(externalSubIndexBase + i), Codec: s.codec,
			Language: s.language, Title: s.title, IsForced: b2i(s.forced),
			IsHearingImpaired: b2i(s.hearingImpaired), ExternalPath: s.path,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
