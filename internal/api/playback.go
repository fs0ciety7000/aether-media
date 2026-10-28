package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/stream"
)

// Playback groups the services needed to stream media.
type Playback struct {
	Streams     *stream.Manager
	Prober      *probe.Prober
	FFmpeg      string
	SubtitleDir string // cache of converted WebVTT files
}

type trackDTO struct {
	Index    int64  `json:"index"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Codec    string `json:"codec"`
	Channels int64  `json:"channels,omitempty"`
	Default  bool   `json:"default"`
	Forced   bool   `json:"forced,omitempty"`
	External bool   `json:"external,omitempty"`
}

type subtitleTrackDTO struct {
	trackDTO
	Delivery string `json:"delivery"` // vtt | burn
	URL      string `json:"url,omitempty"`
}

type playbackVideoDTO struct {
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	HDR    string `json:"hdr,omitempty"`
	Copied bool   `json:"copied"`
}

type playbackInfo struct {
	SessionID        string             `json:"sessionId"`
	Method           string             `json:"method"`
	Reasons          []string           `json:"reasons"`
	URL              string             `json:"url"`
	MediaFileID      int64              `json:"mediaFileId"`
	DurationMs       int64              `json:"durationMs"`
	StartMs          int64              `json:"startMs"`
	AudioTracks      []trackDTO         `json:"audioTracks"`
	SubtitleTracks   []subtitleTrackDTO `json:"subtitleTracks"`
	SelectedAudio    int                `json:"selectedAudio"`
	SelectedSubtitle int                `json:"selectedSubtitle"`
	Video            *playbackVideoDTO  `json:"video,omitempty"`
}

type playbackRequest struct {
	Profile       stream.Profile `json:"profile"`
	MediaFileID   int64          `json:"mediaFileId"`
	AudioIndex    *int           `json:"audioIndex"`
	SubtitleIndex *int           `json:"subtitleIndex"`
	StartMs       int64          `json:"startMs"`
}

func (s *Server) requirePlayback(w http.ResponseWriter) bool {
	if s.Playback == nil || s.Playback.Streams == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "lecture indisponible")
		return false
	}
	return true
}

// startPlayback: POST /api/v1/playback/{id} — decides how to play and opens a session.
func (s *Server) startPlayback(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlayback(w) {
		return
	}
	it, ok := s.loadItem(w, r)
	if !ok {
		return
	}
	var req playbackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	ctx := r.Context()
	files, err := s.store.ListItemMediaFiles(ctx, it.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	var mf *db.MediaFile
	for i := range files {
		if req.MediaFileID == 0 || files[i].ID == req.MediaFileID {
			mf = &files[i]
			break
		}
	}
	if mf == nil {
		writeError(w, http.StatusNotFound, "no_media", "aucun fichier lisible pour cet élément")
		return
	}
	info, err := s.Playback.Prober.Probe(ctx, mf.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "unreadable", "fichier illisible : "+err.Error())
		return
	}
	streams, err := s.store.ListMediaStreams(ctx, mf.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	external := map[int]db.MediaStream{}
	for _, st := range streams {
		if st.IsExternal == 1 {
			external[int(st.Idx)] = st
		}
	}

	sreq := stream.Request{Info: info, Path: mf.Path, AudioIndex: -1, SubtitleIndex: -1, Profile: req.Profile, StartMs: max(0, req.StartMs)}
	if req.AudioIndex != nil {
		sreq.AudioIndex = *req.AudioIndex
	}
	selectedSub := -1
	if req.SubtitleIndex != nil && *req.SubtitleIndex >= 0 {
		selectedSub = *req.SubtitleIndex
		// External subtitles are always delivered as WebVTT, never burnt.
		if _, ok := external[selectedSub]; !ok {
			sreq.SubtitleIndex = selectedSub
		}
	}
	sess, err := s.Playback.Streams.Start(ctx, sreq)
	if errors.Is(err, stream.ErrBusy) {
		writeError(w, http.StatusServiceUnavailable, "transcode_busy", "trop de lectures en cours sur le serveur")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}

	d := sess.Decision
	out := playbackInfo{
		SessionID: sess.ID, Method: string(d.Method), Reasons: d.Reasons, MediaFileID: mf.ID,
		DurationMs: sess.DurationMs, StartMs: sess.StartMs, AudioTracks: []trackDTO{}, SubtitleTracks: []subtitleTrackDTO{},
		SelectedAudio: d.AudioIndex, SelectedSubtitle: selectedSub,
	}
	if out.Reasons == nil {
		out.Reasons = []string{}
	}
	if d.Method == stream.Method("direct") {
		out.URL = fmt.Sprintf("/api/v1/media/%d/stream", mf.ID)
	} else {
		out.URL = fmt.Sprintf("/api/v1/playback/sessions/%s/master.m3u8", sess.ID)
	}
	subURL := func(idx int) string { return fmt.Sprintf("/api/v1/media/%d/subtitles/%d.vtt", mf.ID, idx) }
	for _, st := range info.Streams {
		t := trackDTO{Index: int64(st.Index), Language: st.Language, Title: st.Title, Codec: st.Codec,
			Channels: int64(st.Channels), Default: st.Default, Forced: st.Forced}
		switch st.Type {
		case "audio":
			out.AudioTracks = append(out.AudioTracks, t)
		case "subtitle":
			sub := subtitleTrackDTO{trackDTO: t, Delivery: "vtt", URL: subURL(st.Index)}
			if st.IsImageSub {
				sub.Delivery, sub.URL = "burn", ""
			}
			out.SubtitleTracks = append(out.SubtitleTracks, sub)
		case "video":
			if out.Video == nil && !st.IsAttachedPic {
				out.Video = &playbackVideoDTO{Codec: st.Codec, Width: st.Width, Height: st.Height, HDR: st.HDR, Copied: d.VideoCopy || d.Method == "direct"}
			}
		}
	}
	for idx, st := range external {
		out.SubtitleTracks = append(out.SubtitleTracks, subtitleTrackDTO{
			trackDTO: trackDTO{Index: int64(idx), Language: st.Language, Title: st.Title, Codec: st.Codec, Forced: st.IsForced == 1, External: true},
			Delivery: "vtt", URL: subURL(idx),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) pingPlayback(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlayback(w) {
		return
	}
	if !s.Playback.Streams.Ping(r.PathValue("sid")) {
		writeError(w, http.StatusNotFound, "not_found", "session inconnue ou expirée")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) stopPlayback(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlayback(w) {
		return
	}
	s.Playback.Streams.Stop(r.PathValue("sid"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveHLS(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlayback(w) {
		return
	}
	s.Playback.Streams.ServeHLS(w, r, r.PathValue("sid"), r.PathValue("name"))
}

func (s *Server) loadMediaFile(w http.ResponseWriter, r *http.Request) (db.GetMediaFileRow, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return db.GetMediaFileRow{}, false
	}
	mf, err := s.store.GetMediaFile(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return mf, false
	}
	if err != nil {
		s.internalError(w, err)
		return mf, false
	}
	return mf, true
}

var mediaTypes = map[string]string{
	".mkv": "video/x-matroska", ".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime",
	".webm": "video/webm", ".ts": "video/mp2t", ".m2ts": "video/mp2t", ".avi": "video/x-msvideo",
	".mp3": "audio/mpeg", ".flac": "audio/flac", ".m4a": "audio/mp4", ".ogg": "audio/ogg", ".opus": "audio/ogg",
	".wav": "audio/wav", ".aac": "audio/aac",
}

// mediaStream: GET /api/v1/media/{id}/stream — the raw file with Range support.
func (s *Server) mediaStream(w http.ResponseWriter, r *http.Request) {
	mf, ok := s.loadMediaFile(w, r)
	if !ok {
		return
	}
	f, err := os.Open(mf.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "file_missing", "fichier introuvable sur le disque")
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		s.internalError(w, err)
		return
	}
	ext := strings.ToLower(filepath.Ext(mf.Path))
	ctype := mediaTypes[ext]
	if ctype == "" {
		ctype = mime.TypeByExtension(ext)
	}
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// mediaSubtitle: GET /api/v1/media/{id}/subtitles/{n}.vtt — text subtitle as WebVTT (cached).
func (s *Server) mediaSubtitle(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlayback(w) {
		return
	}
	mf, ok := s.loadMediaFile(w, r)
	if !ok {
		return
	}
	idx, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("file"), ".vtt"))
	if err != nil || !strings.HasSuffix(r.PathValue("file"), ".vtt") {
		http.NotFound(w, r)
		return
	}
	st, err := s.store.GetMediaStream(r.Context(), db.GetMediaStreamParams{MediaFileID: mf.ID, Idx: int64(idx)})
	if err != nil || st.Type != "subtitle" || st.IsImageSub == 1 {
		http.NotFound(w, r)
		return
	}
	src, srcIdx := mf.Path, idx
	if st.IsExternal == 1 {
		src, srcIdx = st.ExternalPath, 0
	}
	fi, err := os.Stat(src)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	out := filepath.Join(s.Playback.SubtitleDir, fmt.Sprintf("%d-%d-%d.vtt", mf.ID, idx, fi.ModTime().Unix()))
	if _, err := os.Stat(out); err != nil {
		if err := os.MkdirAll(s.Playback.SubtitleDir, 0o750); err != nil {
			s.internalError(w, err)
			return
		}
		if err := stream.ExtractSubtitle(r.Context(), s.Playback.FFmpeg, src, srcIdx, out); err != nil {
			s.log.Warn("conversion de sous-titre", "file", src, "index", idx, "err", err)
			writeError(w, http.StatusUnprocessableEntity, "subtitle_failed", "sous-titre illisible")
			return
		}
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, out)
}
