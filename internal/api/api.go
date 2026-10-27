// Package api exposes the HTTP API (/api/v1) and mounts the web UI.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/version"
)

type Server struct {
	store     *store.Store
	log       *slog.Logger
	startedAt time.Time
}

func New(s *store.Store, log *slog.Logger) *Server {
	return &Server{store: s, log: log, startedAt: time.Now().UTC()}
}

// Handler returns the root handler: API routes, health check and the SPA.
func (s *Server) Handler(web http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/system/info", s.systemInfo)
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
