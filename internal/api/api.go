// Package api exposes the HTTP API (/api/v1) and mounts the web UI.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/library"
	"github.com/fs0ciety7000/aether-media/internal/metadata/refresh"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/version"
)

// Deps are the services the API is built on. Scanner, Watcher and Hub may be
// nil in tests that do not exercise them.
type Deps struct {
	Store   *store.Store
	Log     *slog.Logger
	Scanner *library.Scanner
	Watcher *library.Watcher
	Hub     *events.Hub
	// Refresher and Images are optional (nil disables the related routes).
	Refresher *refresh.Refresher
	Images    ImageServer
	Features  Features
	Playback  *Playback
	Sources   *sources.Manager
}

// Features reports optional capabilities to clients.
type Features struct {
	TMDB        bool `json:"tmdb"`
	MusicBrainz bool `json:"musicbrainz"`
}

// ImageServer serves cached image renditions.
type ImageServer interface {
	ServeVariant(w http.ResponseWriter, r *http.Request, hash string, width int)
}

type Server struct {
	Deps
	store     *store.Store
	log       *slog.Logger
	startedAt time.Time
}

func New(d Deps) *Server {
	return &Server{Deps: d, store: d.Store, log: d.Log, startedAt: time.Now().UTC()}
}

// Handler returns the root handler: API routes, health check and the SPA.
func (s *Server) Handler(web http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/system/info", s.systemInfo)
	mux.HandleFunc("GET /api/v1/libraries", s.listLibraries)
	mux.HandleFunc("POST /api/v1/libraries", s.createLibrary)
	mux.HandleFunc("GET /api/v1/libraries/{id}", s.getLibrary)
	mux.HandleFunc("DELETE /api/v1/libraries/{id}", s.deleteLibrary)
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan", s.scanLibrary)
	mux.HandleFunc("POST /api/v1/libraries/{id}/sources", s.addSource)
	mux.HandleFunc("PATCH /api/v1/libraries/{id}/sources/{sid}", s.patchSource)
	mux.HandleFunc("DELETE /api/v1/libraries/{id}/sources/{sid}", s.deleteSource)
	mux.HandleFunc("POST /api/v1/sources/test", s.testSource)
	mux.HandleFunc("GET /internal/vfs/{id}/{exp}/{sig}/{path...}", s.vfsProxy)
	mux.HandleFunc("GET /api/v1/items", s.listItems)
	mux.HandleFunc("GET /api/v1/libraries/{id}/filters", s.libraryFilters)
	mux.HandleFunc("GET /api/v1/home", s.home)
	mux.HandleFunc("GET /api/v1/search", s.search)
	mux.HandleFunc("GET /api/v1/items/{id}/similar", s.similar)
	mux.HandleFunc("GET /api/v1/items/{id}", s.getItem)
	mux.HandleFunc("PATCH /api/v1/items/{id}", s.patchItem)
	mux.HandleFunc("POST /api/v1/items/{id}/refresh", s.refreshItem)
	mux.HandleFunc("POST /api/v1/items/{id}/identify/search", s.identifySearch)
	mux.HandleFunc("POST /api/v1/items/{id}/identify", s.identify)
	mux.HandleFunc("GET /api/v1/items/{id}/images/{type}", s.itemImage)
	mux.HandleFunc("POST /api/v1/playback/{id}", s.startPlayback)
	mux.HandleFunc("POST /api/v1/playback/sessions/{sid}/ping", s.pingPlayback)
	mux.HandleFunc("DELETE /api/v1/playback/sessions/{sid}", s.stopPlayback)
	mux.HandleFunc("GET /api/v1/playback/sessions/{sid}/{name}", s.serveHLS)
	mux.HandleFunc("GET /api/v1/media/{id}/stream", s.mediaStream)
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/{file}", s.mediaSubtitle)
	mux.HandleFunc("GET /api/v1/jobs", s.jobStats)
	mux.HandleFunc("GET /api/v1/events", s.events)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route inconnue")
	})
	mux.Handle("/", web)
	return s.recoverer(s.logRequests(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type systemInfo struct {
	Name          string    `json:"name"`
	Version       string    `json:"version"`
	Commit        string    `json:"commit"`
	StartedAt     time.Time `json:"startedAt"`
	SetupRequired bool      `json:"setupRequired"`
	Metadata      Features  `json:"metadata"`
}

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.CountUsers(r.Context())
	if err != nil {
		s.log.Error("count users", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}
	writeJSON(w, http.StatusOK, systemInfo{
		Name:          "Aether",
		Version:       version.Version,
		Commit:        version.Commit,
		StartedAt:     s.startedAt,
		SetupRequired: users == 0,
		Metadata:      s.Features,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]apiError{"error": {Code: code, Message: msg}})
}
