// Package api exposes the HTTP API (/api/v1) and mounts the web UI.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/library"
	"github.com/fs0ciety7000/aether-media/internal/logbuf"
	"github.com/fs0ciety7000/aether-media/internal/metadata/refresh"
	"github.com/fs0ciety7000/aether-media/internal/settings"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/syncsvc"
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
	// Sync is the Trakt / Simkl synchronisation (nil disables the routes).
	Sync *syncsvc.Service
	// BrowseRoots limits the server-side folder picker (nil = OS defaults).
	BrowseRoots []string
	// Settings are the editable server settings (nil disables the admin
	// settings, the assistant state and the system status).
	Settings *settings.Service
	// Runner is the job queue (admin "Tâches": cancel, retry).
	Runner *jobs.Runner
	// Logs is the in-memory log buffer (admin "Journaux").
	Logs *logbuf.Buffer
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
	authState *authState
	plays     *playTracker
	probes    probeCache
	jobsNote  jobsNotifier
}

func New(d Deps) *Server {
	s := &Server{Deps: d, store: d.Store, log: d.Log, startedAt: time.Now().UTC(), authState: newAuthState(), plays: newPlayTracker()}
	if d.Runner != nil && d.Hub != nil {
		d.Runner.SetNotify(s.jobsChanged)
	}
	return s
}

// Handler returns the root handler: API routes, health check and the SPA.
func (s *Server) Handler(web http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/system/info", s.systemInfo)
	// Accounts.
	mux.HandleFunc("POST /api/v1/setup", s.setup)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/me", s.me)
	mux.HandleFunc("PATCH /api/v1/me", s.patchMe)
	mux.HandleFunc("GET /api/v1/me/sessions", s.mySessions)
	mux.HandleFunc("DELETE /api/v1/me/sessions/{sid}", s.revokeSession)
	mux.HandleFunc("GET /api/v1/users", s.admin(s.listUsers))
	mux.HandleFunc("POST /api/v1/users", s.admin(s.postUser))
	mux.HandleFunc("PATCH /api/v1/users/{uid}", s.admin(s.patchUser))
	mux.HandleFunc("DELETE /api/v1/users/{uid}", s.admin(s.deleteUser))
	// Libraries and sources.
	mux.HandleFunc("GET /api/v1/libraries", s.listLibraries)
	mux.HandleFunc("POST /api/v1/libraries", s.admin(s.createLibrary))
	mux.HandleFunc("GET /api/v1/libraries/{id}", s.getLibrary)
	mux.HandleFunc("DELETE /api/v1/libraries/{id}", s.admin(s.deleteLibrary))
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan", s.admin(s.scanLibrary))
	mux.HandleFunc("POST /api/v1/libraries/{id}/sources", s.admin(s.addSource))
	mux.HandleFunc("PATCH /api/v1/libraries/{id}/sources/{sid}", s.admin(s.patchSource))
	mux.HandleFunc("DELETE /api/v1/libraries/{id}/sources/{sid}", s.admin(s.deleteSource))
	mux.HandleFunc("POST /api/v1/libraries/{id}/sources/{sid}/test", s.admin(s.testStoredSource))
	mux.HandleFunc("POST /api/v1/sources/test", s.admin(s.testSource))
	mux.HandleFunc("GET /api/v1/fs/browse", s.admin(s.browseLocal))
	mux.HandleFunc("GET /api/v1/agents", s.admin(s.listAgents))
	mux.HandleFunc("POST /api/v1/agents/discover", s.admin(s.discoverAgents))
	mux.HandleFunc("POST /api/v1/agents/pair", s.admin(s.pairAgent))
	mux.HandleFunc("DELETE /api/v1/agents/{aid}", s.admin(s.deleteAgent))
	mux.HandleFunc("GET /api/v1/agents/{aid}/browse", s.admin(s.agentBrowse))
	mux.HandleFunc("GET /api/v1/agents/{aid}/shares", s.admin(s.agentShares))
	mux.HandleFunc("POST /api/v1/agents/{aid}/shares", s.admin(s.agentShares))
	mux.HandleFunc("GET /api/v1/libraries/{id}/filters", s.libraryFilters)
	mux.HandleFunc("GET /internal/vfs/{id}/{exp}/{sig}/{path...}", s.vfsProxy)
	// Browsing.
	mux.HandleFunc("GET /api/v1/items", s.listItems)
	mux.HandleFunc("GET /api/v1/home", s.home)
	mux.HandleFunc("GET /api/v1/search", s.search)
	mux.HandleFunc("GET /api/v1/items/{id}/similar", s.similar)
	mux.HandleFunc("GET /api/v1/items/{id}", s.getItem)
	mux.HandleFunc("GET /api/v1/items/{id}/images/{type}", s.itemImage)
	mux.HandleFunc("PUT /api/v1/items/{id}/userdata", s.putUserData)
	mux.HandleFunc("PATCH /api/v1/items/{id}", s.admin(s.patchItem))
	mux.HandleFunc("POST /api/v1/items/{id}/refresh", s.admin(s.refreshItem))
	mux.HandleFunc("POST /api/v1/items/{id}/identify/search", s.admin(s.identifySearch))
	mux.HandleFunc("POST /api/v1/items/{id}/identify", s.admin(s.identify))
	// Collections.
	mux.HandleFunc("GET /api/v1/collections", s.listCollections)
	mux.HandleFunc("POST /api/v1/collections", s.createCollection)
	mux.HandleFunc("GET /api/v1/collections/{cid}/items", s.collectionItems)
	mux.HandleFunc("PATCH /api/v1/collections/{cid}", s.patchCollection)
	mux.HandleFunc("DELETE /api/v1/collections/{cid}", s.deleteCollection)
	mux.HandleFunc("PUT /api/v1/collections/{cid}/items/{id}", s.addToCollection)
	mux.HandleFunc("DELETE /api/v1/collections/{cid}/items/{id}", s.removeFromCollection)
	// Playback.
	mux.HandleFunc("POST /api/v1/playback/{id}", s.startPlayback)
	mux.HandleFunc("POST /api/v1/playback/sessions/{sid}/ping", s.pingPlayback)
	mux.HandleFunc("DELETE /api/v1/playback/sessions/{sid}", s.stopPlayback)
	mux.HandleFunc("GET /api/v1/playback/sessions/{sid}/{name}", s.serveHLS)
	mux.HandleFunc("GET /api/v1/media/{id}/stream", s.mediaStream)
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/{file}", s.mediaSubtitle)
	// Trakt / Simkl synchronisation and watchlist.
	mux.HandleFunc("GET /api/v1/me/sync", s.syncList)
	mux.HandleFunc("PATCH /api/v1/me/sync/{service}", s.syncPatch)
	mux.HandleFunc("DELETE /api/v1/me/sync/{service}", s.syncDisconnect)
	mux.HandleFunc("POST /api/v1/me/sync/{service}/connect", s.syncConnect)
	mux.HandleFunc("POST /api/v1/me/sync/{service}/poll", s.syncPoll)
	mux.HandleFunc("POST /api/v1/me/sync/{service}/run", s.syncRun)
	mux.HandleFunc("GET /api/v1/me/sync/{service}/events", s.syncEvents)
	mux.HandleFunc("GET /api/v1/sync/apps", s.admin(s.syncApps))
	mux.HandleFunc("PUT /api/v1/sync/apps/{service}", s.admin(s.syncSetApp))
	mux.HandleFunc("DELETE /api/v1/sync/apps/{service}", s.admin(s.syncDeleteApp))
	mux.HandleFunc("GET /api/v1/watchlist", s.watchlist)
	mux.HandleFunc("PUT /api/v1/watchlist/{id}", s.watchlistAdd)
	mux.HandleFunc("DELETE /api/v1/watchlist/{id}", s.watchlistRemove)
	mux.HandleFunc("GET /api/v1/jobs", s.admin(s.jobStats))
	mux.HandleFunc("GET /api/v1/events", s.events)
	s.adminRoutes(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route inconnue")
	})
	mux.Handle("/", web)
	return s.recoverer(s.logRequests(s.authenticate(mux)))
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
	Sync          syncInfo  `json:"sync"`
	// ServerName is the name chosen by the administrator.
	ServerName string `json:"serverName"`
	// SetupWizardPending (administrators only): the first-run assistant was
	// left before its end and should resume.
	SetupWizardPending bool `json:"setupWizardPending,omitempty"`
}

type syncInfo struct {
	Trakt bool `json:"trakt"`
	Simkl bool `json:"simkl"`
}

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.CountUsers(r.Context())
	if err != nil {
		s.log.Error("count users", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}
	info := systemInfo{
		Name:          "Aether",
		Version:       version.Version,
		Commit:        version.Commit,
		StartedAt:     s.startedAt,
		SetupRequired: users == 0,
		Metadata:      s.Features,
		Sync:          syncInfo{Trakt: s.Sync != nil && s.Sync.Available(r.Context(), "trakt"), Simkl: s.Sync != nil && s.Sync.Available(r.Context(), "simkl")},
		ServerName:    "Aether",
	}
	if s.Settings != nil {
		info.Metadata.TMDB = s.Settings.TMDBEnabled()
		info.ServerName = s.Settings.Current().ServerName
		if users > 0 && isAdmin(r) {
			info.SetupWizardPending = s.Settings.WizardPending(r.Context())
		}
	}
	writeJSON(w, http.StatusOK, info)
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
