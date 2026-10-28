package library

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
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
func findSidecars(videoPath string) []sidecar {
	dir := filepath.Dir(videoPath)
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []sidecar
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
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
func saveSidecars(ctx context.Context, q *db.Queries, mediaFileID int64, videoPath string) error {
	if err := q.DeleteExternalStreams(ctx, mediaFileID); err != nil {
		return err
	}
	for i, s := range findSidecars(videoPath) {
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
