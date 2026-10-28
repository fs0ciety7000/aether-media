package api

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/logbuf"
	"github.com/fs0ciety7000/aether-media/internal/metadata/tmdb"
	"github.com/fs0ciety7000/aether-media/internal/settings"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/stream"
	"github.com/fs0ciety7000/aether-media/internal/version"
)

// Admin (M7): server settings, job queue, logs, playback sessions.

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/settings", s.admin(s.getSettings))
	mux.HandleFunc("PATCH /api/v1/settings", s.admin(s.patchSettings))
	mux.HandleFunc("POST /api/v1/settings/tmdb/check", s.admin(s.checkTMDB))
	mux.HandleFunc("GET /api/v1/system/status", s.admin(s.systemStatus))
	mux.HandleFunc("GET /api/v1/jobs/queue", s.admin(s.jobQueue))
	mux.HandleFunc("POST /api/v1/jobs/run", s.admin(s.runTask))
	mux.HandleFunc("POST /api/v1/jobs/{jid}/retry", s.admin(s.retryJob))
	mux.HandleFunc("DELETE /api/v1/jobs/{jid}", s.admin(s.cancelJob))
	mux.HandleFunc("GET /api/v1/logs", s.admin(s.listLogs))
	mux.HandleFunc("GET /api/v1/logs/stream", s.admin(s.streamLogs))
	mux.HandleFunc("GET /api/v1/logs/download", s.admin(s.downloadLogs))
	mux.HandleFunc("GET /api/v1/playback/sessions", s.admin(s.listPlaybackSessions))
	mux.HandleFunc("POST /api/v1/playback/sessions/{sid}/stop", s.admin(s.adminStopPlayback))
}

// ---- settings ----

func (s *Server) requireSettings(w http.ResponseWriter) bool {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "réglages indisponibles")
		return false
	}
	return true
}

// getSettings: GET /api/v1/settings (admin)
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettings(w) {
		return
	}
	v, err := s.Settings.View(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// patchSettings: PATCH /api/v1/settings (admin)
func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettings(w) {
		return
	}
	var p settings.Patch
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	v, err := s.Settings.Apply(r.Context(), p)
	var managed *settings.ErrManaged
	var invalid *settings.ErrInvalid
	switch {
	case errors.As(err, &managed):
		writeJSON(w, http.StatusConflict, map[string]any{"error": map[string]string{
			"code": "managed_by_config", "message": managed.Error(), "field": managed.Field}})
		return
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{
			"code": "invalid_setting", "message": invalid.Error(), "field": invalid.Field}})
		return
	case err != nil:
		s.internalError(w, err)
		return
	}
	s.log.Info("réglages du serveur modifiés", "by", principalFrom(r).user.Name, "fields", patchedFields(p))
	writeJSON(w, http.StatusOK, v)
}

func patchedFields(p settings.Patch) string {
	var f []string
	add := func(ok bool, name string) {
		if ok {
			f = append(f, name)
		}
	}
	add(p.ServerName != nil, "serverName")
	add(p.MetadataLanguage != nil, "metadataLanguage")
	add(p.MetadataCountry != nil, "metadataCountry")
	add(p.TMDBAPIKey != nil, "tmdbApiKey")
	add(p.HWAccel != nil, "hwAccel")
	add(p.MaxTranscodes != nil, "maxTranscodes")
	add(p.ScanIntervalMinutes != nil, "scanIntervalMinutes")
	add(p.Wizard != nil, "wizard")
	return strings.Join(f, ",")
}

type tmdbCheckResult struct {
	Valid   bool   `json:"valid"`
	Error   string `json:"error,omitempty"` // invalid_key | unreachable | missing
	Message string `json:"message,omitempty"`
}

// checkTMDB: POST /api/v1/settings/tmdb/check { apiKey? } (admin) — the key
// in effect when apiKey is absent.
func (s *Server) checkTMDB(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettings(w) {
		return
	}
	var req struct {
		APIKey string `json:"apiKey"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
			return
		}
	}
	if strings.TrimSpace(req.APIKey) == "" && !s.Settings.TMDBEnabled() {
		writeJSON(w, http.StatusOK, tmdbCheckResult{Error: "missing", Message: "aucune clé TMDB"})
		return
	}
	err := s.Settings.CheckTMDB(r.Context(), req.APIKey)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, tmdbCheckResult{Valid: true})
	case errors.Is(err, tmdb.ErrInvalidKey):
		writeJSON(w, http.StatusOK, tmdbCheckResult{Error: "invalid_key", Message: "TMDB refuse cette clé"})
	case errors.Is(err, settings.ErrNoChecker):
		writeError(w, http.StatusServiceUnavailable, "unavailable", "vérification indisponible")
	default:
		s.log.Warn("vérification de la clé TMDB", "err", err)
		writeJSON(w, http.StatusOK, tmdbCheckResult{Error: "unreachable", Message: "TMDB est injoignable depuis le serveur"})
	}
}

// ---- system status ----

type binaryStatus struct {
	Path    string `json:"path"`
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type systemStatusDTO struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Commit    string    `json:"commit"`
	GoVersion string    `json:"goVersion"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	StartedAt time.Time `json:"startedAt"`
	Paths     struct {
		Data         string `json:"data"`
		Cache        string `json:"cache"`
		Transcode    string `json:"transcode"`
		Database     string `json:"database"`
		DatabaseSize int64  `json:"databaseSize"`
	} `json:"paths"`
	FFmpeg  binaryStatus `json:"ffmpeg"`
	FFprobe binaryStatus `json:"ffprobe"`
	HWAccel struct {
		Configured string    `json:"configured"`       // none | auto | vaapi | ...
		Device     string    `json:"device,omitempty"` // VAAPI / QSV render node
		Active     string    `json:"active"`           // mode used by new transcodes
		Detected   []string  `json:"detected"`         // working hardware H.264 encoders
		CheckedAt  time.Time `json:"checkedAt"`
	} `json:"hwAccel"`
	Transcodes struct {
		Max    int `json:"max"`
		Active int `json:"active"`
	} `json:"transcodes"`
	Logs struct {
		Capacity int `json:"capacity"`
		Held     int `json:"held"`
	} `json:"logs"`
}

// probeCache memoises expensive checks (FFmpeg version, hardware encoders).
type probeCache struct {
	mu        sync.Mutex
	at        time.Time
	ffmpeg    binaryStatus
	ffprobe   binaryStatus
	hw        []string
	hwChecked time.Time
}

func binaryVersion(ctx context.Context, bin string) binaryStatus {
	st := binaryStatus{Path: bin}
	if bin == "" {
		st.Error = "non configuré"
		return st
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-version").Output()
	if err != nil {
		st.Error = "introuvable ou ne démarre pas"
		return st
	}
	line, _, _ := strings.Cut(string(out), "\n")
	// "ffmpeg version 6.1.1-3ubuntu5 Copyright (c) ..." → "6.1.1-3ubuntu5"
	f := strings.Fields(line)
	if len(f) >= 3 && f[1] == "version" {
		st.Version = f[2]
	} else {
		st.Version = strings.TrimSpace(line)
	}
	st.OK = true
	return st
}

// systemStatus: GET /api/v1/system/status[?detect=true] (admin)
func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireSettings(w) {
		return
	}
	cfg := s.Settings.Config()
	cur := s.Settings.Current()
	var out systemStatusDTO
	out.Name, out.Version, out.Commit = cur.ServerName, version.Version, version.Commit
	out.GoVersion, out.OS, out.Arch, out.StartedAt = runtime.Version(), runtime.GOOS, runtime.GOARCH, s.startedAt
	out.Paths.Data, out.Paths.Cache, out.Paths.Transcode = cfg.Paths.Data, cfg.Paths.Cache, cfg.Paths.Transcode
	out.Paths.Database = cfg.DatabasePath()
	if fi, err := os.Stat(out.Paths.Database); err == nil {
		out.Paths.DatabaseSize = fi.Size()
	}

	pc := &s.probes
	pc.mu.Lock()
	if time.Since(pc.at) > 5*time.Minute {
		pc.ffmpeg = binaryVersion(r.Context(), cfg.Paths.FFmpeg)
		pc.ffprobe = binaryVersion(r.Context(), cfg.Paths.FFprobe)
		pc.at = time.Now()
	}
	if pc.ffmpeg.OK && (pc.hwChecked.IsZero() || r.URL.Query().Get("detect") == "true") {
		pc.hw = stream.DetectHWAccel(r.Context(), cfg.Paths.FFmpeg)
		pc.hwChecked = time.Now().UTC()
	}
	out.FFmpeg, out.FFprobe = pc.ffmpeg, pc.ffprobe
	out.HWAccel.Detected = append([]string{}, pc.hw...)
	out.HWAccel.CheckedAt = pc.hwChecked
	pc.mu.Unlock()

	// What runs now (a change of the setting waits for a restart).
	out.HWAccel.Configured, out.HWAccel.Device = cfg.Transcode.HWAccel, cfg.Transcode.VAAPIDevice
	out.HWAccel.Active = cfg.Transcode.HWAccel
	if s.Playback != nil && s.Playback.Streams != nil {
		if pc.ffmpeg.OK {
			out.HWAccel.Active = s.Playback.Streams.HWAccel()
		}
		out.Transcodes.Max, out.Transcodes.Active = s.Playback.Streams.Limits()
	}
	if s.Logs != nil {
		_, out.Logs.Held = s.Logs.Entries(logbuf.Filter{MinLevel: -100, Limit: 1})
		out.Logs.Capacity = s.Logs.Capacity()
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- jobs ----

type jobDTO struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Key       string    `json:"key"`
	Label     string    `json:"label,omitempty"`
	Status    string    `json:"status"` // running | queued | failed
	Priority  int64     `json:"priority"`
	Attempts  int64     `json:"attempts"`
	Error     string    `json:"error,omitempty"`
	RunAfter  time.Time `json:"runAfter"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type jobQueueDTO struct {
	Running []jobDTO                    `json:"running"`
	Queued  []jobDTO                    `json:"queued"`
	Failed  []jobDTO                    `json:"failed"`
	Counts  map[string]int64            `json:"counts"` // per status
	ByKind  map[string]map[string]int64 `json:"byKind"`
}

func toJobDTO(j db.ListJobsByStatusRow) jobDTO {
	d := jobDTO{ID: j.ID, Kind: j.Kind, Key: j.Key, Label: j.Label, Status: j.Status, Priority: j.Priority, Attempts: j.Attempts,
		Error: logbuf.Redact(j.Error), RunAfter: time.Unix(j.RunAfter, 0).UTC(), CreatedAt: time.Unix(j.CreatedAt, 0).UTC(), UpdatedAt: time.Unix(j.UpdatedAt, 0).UTC()}
	switch j.Kind {
	case "media.probe":
		d.Label = path.Base(strings.ReplaceAll(d.Label, "\\", "/"))
	case "sync.run":
		if svc, user, ok := strings.Cut(d.Label, ":"); ok {
			d.Label = map[string]string{"trakt": "Trakt", "simkl": "Simkl"}[svc] + " · " + user
		}
	}
	if d.Label == "." {
		d.Label = ""
	}
	return d
}

// jobQueue: GET /api/v1/jobs/queue[?kind=&limit=] (admin) — running jobs,
// the head of the queue and recent failures.
func (s *Server) jobQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kind := sql.NullString{String: r.URL.Query().Get("kind"), Valid: r.URL.Query().Get("kind") != ""}
	limit := int64(100)
	if n, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64); err == nil && n > 0 && n <= 500 {
		limit = n
	}
	out := jobQueueDTO{Running: []jobDTO{}, Queued: []jobDTO{}, Failed: []jobDTO{}, Counts: map[string]int64{"running": 0, "queued": 0, "failed": 0}, ByKind: map[string]map[string]int64{}}
	for status, dst := range map[string]*[]jobDTO{"running": &out.Running, "queued": &out.Queued, "failed": &out.Failed} {
		lim := limit
		if status == "failed" {
			lim = min(limit, 50)
		}
		rows, err := s.store.ListJobsByStatus(ctx, db.ListJobsByStatusParams{Status: status, Kind: kind, Lim: lim})
		if err != nil {
			s.internalError(w, err)
			return
		}
		for _, j := range rows {
			*dst = append(*dst, toJobDTO(j))
		}
	}
	stats, err := s.store.JobStats(ctx)
	if err != nil {
		s.internalError(w, err)
		return
	}
	for _, st := range stats {
		out.Counts[st.Status] += st.N
		if out.ByKind[st.Kind] == nil {
			out.ByKind[st.Kind] = map[string]int64{}
		}
		out.ByKind[st.Kind][st.Status] = st.N
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) jobID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if s.Runner == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "tâches indisponibles")
		return 0, false
	}
	id, err := strconv.ParseInt(r.PathValue("jid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "tâche introuvable")
		return 0, false
	}
	return id, true
}

// retryJob: POST /api/v1/jobs/{jid}/retry (admin)
func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	id, ok := s.jobID(w, r)
	if !ok {
		return
	}
	switch err := s.Runner.Retry(r.Context(), id); {
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "tâche introuvable")
	case errors.Is(err, jobs.ErrNotRetryable):
		writeError(w, http.StatusConflict, "not_retryable", "seule une tâche en échec peut être relancée (ou la même tâche est déjà en file)")
	case err != nil:
		s.internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// cancelJob: DELETE /api/v1/jobs/{jid} (admin) — cancels a running or
// queued job, or dismisses a failed one.
func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, ok := s.jobID(w, r)
	if !ok {
		return
	}
	switch err := s.Runner.Cancel(r.Context(), id); {
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "tâche introuvable")
	case errors.Is(err, jobs.ErrNotCancelable):
		writeError(w, http.StatusConflict, "not_cancellable", "cette tâche ne peut plus être annulée")
	case err != nil:
		s.internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// runTask: POST /api/v1/jobs/run { task: "scan" | "metadata" | "sync" } (admin)
func (s *Server) runTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task string `json:"task"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	ctx := r.Context()
	n := 0
	switch req.Task {
	case "scan":
		if s.Scanner == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "analyse indisponible")
			return
		}
		libs, err := s.store.ListLibraries(ctx)
		if err != nil {
			s.internalError(w, err)
			return
		}
		for _, l := range libs {
			ok, err := s.Scanner.EnqueueScan(ctx, l.ID, true)
			if err != nil {
				s.internalError(w, err)
				return
			}
			if ok {
				n++
			}
		}
	case "metadata":
		if s.Refresher == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "métadonnées indisponibles")
			return
		}
		var err error
		if n, err = s.Refresher.RefreshAll(ctx); err != nil {
			s.internalError(w, err)
			return
		}
	case "sync":
		if s.Sync == nil {
			writeError(w, http.StatusServiceUnavailable, "sync_unavailable", "synchronisation indisponible")
			return
		}
		n = s.Sync.EnqueueAll(ctx)
	default:
		writeError(w, http.StatusBadRequest, "invalid_task", "tâche inconnue (scan, metadata, sync)")
		return
	}
	s.log.Info("tâche lancée manuellement", "task", req.Task, "queued", n, "by", principalFrom(r).user.Name)
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": n})
}

// jobsNotifier publishes "jobs.changed" to admins, at most every 700 ms.
type jobsNotifier struct {
	mu      sync.Mutex
	pending bool
}

func (s *Server) jobsChanged() {
	n := &s.jobsNote
	n.mu.Lock()
	if n.pending {
		n.mu.Unlock()
		return
	}
	n.pending = true
	n.mu.Unlock()
	time.AfterFunc(700*time.Millisecond, func() {
		n.mu.Lock()
		n.pending = false
		n.mu.Unlock()
		counts := map[string]int64{"running": 0, "queued": 0, "failed": 0}
		stats, err := s.store.JobStats(context.Background())
		if err != nil {
			return
		}
		for _, st := range stats {
			counts[st.Status] += st.N
		}
		s.Hub.PublishAdmin("jobs.changed", counts)
	})
}

// ---- logs ----

func logFilter(r *http.Request) logbuf.Filter {
	q := r.URL.Query()
	f := logbuf.Filter{MinLevel: logbuf.ParseLevel(q.Get("level")), Query: strings.TrimSpace(q.Get("q"))}
	if n, err := strconv.ParseInt(q.Get("after"), 10, 64); err == nil {
		f.After = n
	}
	f.Limit = 500
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= logbuf.DefaultCapacity {
		f.Limit = n
	}
	return f
}

func (s *Server) requireLogs(w http.ResponseWriter) bool {
	if s.Logs == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "journaux indisponibles")
		return false
	}
	return true
}

// listLogs: GET /api/v1/logs?level=&q=&after=&limit= (admin), oldest first.
func (s *Server) listLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireLogs(w) {
		return
	}
	entries, held := s.Logs.Entries(logFilter(r))
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "held": held, "capacity": s.Logs.Capacity()})
}

// streamLogs: GET /api/v1/logs/stream?level=&q= (admin, SSE "log" events).
// Separate from /events so that a burst of logs never crowds out the
// other events.
func (s *Server) streamLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireLogs(w) {
		return
	}
	f := logFilter(r)
	f.After, f.Limit = 0, 0
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 5000\n\n")
	if err := rc.Flush(); err != nil {
		return
	}
	ch, unsubscribe := s.Logs.Subscribe()
	defer unsubscribe()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": ping\n\n")
		case e := <-ch:
			if !f.Match(e) {
				continue
			}
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", e.Seq, data)
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}

// downloadLogs: GET /api/v1/logs/download?level=&q= (admin), plain text.
func (s *Server) downloadLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireLogs(w) {
		return
	}
	f := logFilter(r)
	f.Limit = 0
	entries, _ := s.Logs.Entries(f)
	name := "aether-journaux-" + time.Now().Format("20060102-150405") + ".log"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# Aether %s (%s) — %d entrées, secrets masqués\n", version.Version, version.Commit, len(entries))
	for _, e := range entries {
		_, _ = bw.WriteString(logbuf.Format(e) + "\n")
	}
	_ = bw.Flush()
}

// ---- playback sessions ----

type activeSessionDTO struct {
	ID         string    `json:"id"`
	User       *userRef  `json:"user,omitempty"`
	Item       *itemRef  `json:"item,omitempty"`
	Method     string    `json:"method"`  // direct | remux | transcode
	Reasons    []string  `json:"reasons"` // why not direct
	PositionMs int64     `json:"positionMs"`
	DurationMs int64     `json:"durationMs"`
	Paused     bool      `json:"paused"`
	Bitrate    int64     `json:"bitrate"` // bits/s (estimate)
	Video      *avDTO    `json:"video,omitempty"`
	Audio      *avDTO    `json:"audio,omitempty"`
	HWAccel    string    `json:"hwAccel,omitempty"` // video encoder: none | vaapi | ...
	StartedAt  time.Time `json:"startedAt"`
	LastActive time.Time `json:"lastActive"`
}

type userRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type itemRef struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	SeriesTitle string `json:"seriesTitle,omitempty"`
	Season      int64  `json:"season,omitempty"`
	Episode     int64  `json:"episode,omitempty"`
	Year        int64  `json:"year,omitempty"`
}

type avDTO struct {
	Codec  string `json:"codec"`
	Copied bool   `json:"copied"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// snapshot returns a copy of the tracked plays.
func (t *playTracker) snapshot() map[string]play {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]play, len(t.plays))
	for id, p := range t.plays {
		out[id] = *p
	}
	return out
}

// listPlaybackSessions: GET /api/v1/playback/sessions (admin)
func (s *Server) listPlaybackSessions(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlayback(w) {
		return
	}
	ctx := r.Context()
	plays := s.plays.snapshot()
	users := map[int64]*userRef{}
	out := []activeSessionDTO{}
	for _, si := range s.Playback.Streams.Sessions() {
		d := activeSessionDTO{ID: si.ID, Method: string(si.Decision.Method), Reasons: si.Decision.Reasons, DurationMs: si.DurationMs,
			PositionMs: si.StartMs, Bitrate: si.Bitrate, HWAccel: si.HW, StartedAt: si.Started.UTC(), LastActive: si.LastActive.UTC()}
		if d.Reasons == nil {
			d.Reasons = []string{}
		}
		direct := si.Decision.Method == stream.MethodDirect
		if si.VideoCodec != "" {
			d.Video = &avDTO{Codec: si.VideoCodec, Copied: direct || si.Decision.VideoCopy, Width: si.Width, Height: si.Height}
			if !d.Video.Copied {
				d.Video.Codec = "h264"
			}
		}
		if si.AudioCodec != "" {
			d.Audio = &avDTO{Codec: si.AudioCodec, Copied: direct || si.Decision.AudioCopy}
			if !d.Audio.Copied {
				d.Audio.Codec = "aac"
			}
		}
		if p, ok := plays[si.ID]; ok {
			d.PositionMs, d.Paused = p.positionMs, p.paused
			u, ok := users[p.userID]
			if !ok {
				if row, err := s.store.GetUser(ctx, p.userID); err == nil {
					u = &userRef{ID: row.ID, Name: row.Name}
				}
				users[p.userID] = u
			}
			d.User = u
			d.Item = s.itemRef(ctx, p.itemID)
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) itemRef(ctx context.Context, id int64) *itemRef {
	it, err := s.store.GetItem(ctx, id)
	if err != nil {
		return nil
	}
	ref := &itemRef{ID: it.ID, Kind: it.Kind, Title: it.Title, Year: it.Year.Int64}
	if it.Kind == "episode" {
		ref.Season, ref.Episode = it.ParentIndexNumber.Int64, it.IndexNumber.Int64
		parent := it.ParentID
		for range 2 { // episode → season → series
			if !parent.Valid {
				break
			}
			p, err := s.store.GetItem(ctx, parent.Int64)
			if err != nil {
				break
			}
			if p.Kind == "series" {
				ref.SeriesTitle = p.Title
				break
			}
			parent = p.ParentID
		}
	}
	return ref
}

// adminStopPlayback: POST /api/v1/playback/sessions/{sid}/stop (admin) —
// ends someone's playback (the position reached is kept).
func (s *Server) adminStopPlayback(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlayback(w) {
		return
	}
	sid := r.PathValue("sid")
	if _, ok := s.Playback.Streams.Get(sid); !ok {
		writeError(w, http.StatusNotFound, "not_found", "session inconnue ou terminée")
		return
	}
	plays := s.plays.snapshot()
	p, tracked := plays[sid]
	s.plays.end(sid)
	s.Playback.Streams.Stop(sid)
	if tracked {
		if s.Hub != nil {
			s.Hub.PublishUser(p.userID, "playback.stopped", map[string]string{"sessionId": sid, "reason": "admin"})
		}
		if !p.finished && p.positionMs > 0 {
			s.localChanged(r, p.userID)
		}
	}
	s.log.Info("lecture arrêtée par un administrateur", "session", sid, "by", principalFrom(r).user.Name)
	s.playbackChanged()
	w.WriteHeader(http.StatusNoContent)
}

// playbackChanged tells admins that the list of sessions changed.
func (s *Server) playbackChanged() {
	if s.Hub != nil {
		s.Hub.PublishAdmin("playback.sessions", nil)
	}
}
