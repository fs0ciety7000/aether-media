package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/library"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

type sourceDTO struct {
	ID        int64                `json:"id"`
	Kind      string               `json:"kind"`
	Name      string               `json:"name"`
	Root      string               `json:"root"`
	Status    string               `json:"status"`
	LastError string               `json:"lastError,omitempty"`
	LastSeen  *time.Time           `json:"lastSeen,omitempty"`
	Agent     *sources.AgentConfig `json:"agent,omitempty"`
	SFTP      *sources.SFTPConfig  `json:"sftp,omitempty"`
}

func toSourceDTO(src db.LibrarySource) sourceDTO {
	dto := sourceDTO{ID: src.ID, Kind: src.Kind, Name: src.Name, Root: src.Root, Status: src.Status, LastError: src.LastError}
	if src.LastSeen.Valid {
		t := time.Unix(src.LastSeen.Int64, 0).UTC()
		dto.LastSeen = &t
	}
	var cfg sources.Config
	if json.Unmarshal([]byte(src.Config), &cfg) == nil {
		dto.Agent, dto.SFTP = cfg.Agent, cfg.SFTP
	}
	return dto
}

type libraryDTO struct {
	ID        int64            `json:"id"`
	Name      string           `json:"name"`
	Kind      string           `json:"kind"`
	Paths     []string         `json:"paths"`
	Sources   []sourceDTO      `json:"sources"`
	Counts    map[string]int64 `json:"counts"`
	CreatedAt time.Time        `json:"createdAt"`
}

func (s *Server) libraryDTO(r *http.Request, l db.Library) (libraryDTO, error) {
	dto := libraryDTO{ID: l.ID, Name: l.Name, Kind: l.Kind, Paths: []string{}, Sources: []sourceDTO{}, Counts: map[string]int64{},
		CreatedAt: time.Unix(l.CreatedAt, 0).UTC()}
	srcs, err := s.store.ListSources(r.Context(), l.ID)
	if err != nil {
		return dto, err
	}
	if !isAdmin(r) {
		srcs = nil // where files live is administrators' business
	}
	for _, src := range srcs {
		dto.Sources = append(dto.Sources, toSourceDTO(src))
		if src.Kind == sources.KindLocal {
			dto.Paths = append(dto.Paths, src.Root)
		}
	}
	stats, err := s.store.LibraryStats(r.Context(), l.ID)
	if err != nil {
		return dto, err
	}
	for _, st := range stats {
		dto.Counts[st.Kind] = st.N
	}
	return dto, nil
}

// visibleLibraries lists the libraries the requesting user may browse.
func (s *Server) visibleLibraries(r *http.Request) ([]db.Library, error) {
	libs, err := s.store.ListLibraries(r.Context())
	if err != nil {
		return nil, err
	}
	allowed := viewer(r).LibraryIDs
	if allowed == nil {
		return libs, nil
	}
	out := libs[:0]
	for _, l := range libs {
		if slices.Contains(allowed, l.ID) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *Server) listLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.visibleLibraries(r)
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := make([]libraryDTO, 0, len(libs))
	for _, l := range libs {
		dto, err := s.libraryDTO(r, l)
		if err != nil {
			s.internalError(w, err)
			return
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLibrary(w, r)
	if !ok {
		return
	}
	if allowed := viewer(r).LibraryIDs; allowed != nil && !slices.Contains(allowed, l.ID) {
		writeError(w, http.StatusNotFound, "not_found", "bibliothèque introuvable")
		return
	}
	dto, err := s.libraryDTO(r, l)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

type createLibraryRequest struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Paths   []string        `json:"paths"`
	Sources []sources.Input `json:"sources"`
}

var libraryKinds = map[string]bool{
	library.KindMovies: true, library.KindShows: true, library.KindMusic: true, library.KindMixed: true,
}

func (s *Server) requireSources(w http.ResponseWriter) bool {
	if s.Sources == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "gestion des sources indisponible")
		return false
	}
	return true
}

func (s *Server) createLibrary(w http.ResponseWriter, r *http.Request) {
	if !s.requireSources(w) {
		return
	}
	var req createLibraryRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "nom requis (100 caractères max)")
		return
	}
	if !libraryKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, "invalid_kind", "type attendu : movies, shows, music ou mixed")
		return
	}
	inputs := req.Sources
	for _, p := range req.Paths {
		inputs = append(inputs, sources.Input{Kind: sources.KindLocal, Root: p})
	}
	if len(inputs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_sources", "au moins une source est requise")
		return
	}
	if !s.checkLocalOverlap(w, r, inputs) {
		return
	}

	var lib db.Library
	err := s.store.Tx(r.Context(), func(q *db.Queries) error {
		var err error
		if lib, err = q.CreateLibrary(r.Context(), db.CreateLibraryParams{Name: req.Name, Kind: req.Kind}); err != nil {
			return err
		}
		for _, in := range inputs {
			if _, err := s.Sources.Create(r.Context(), q, lib.ID, in); err != nil {
				return err
			}
		}
		return nil
	})
	if s.sourceError(w, err) {
		return
	}
	s.afterLibraryChange(r)
	s.enqueueScan(r, lib.ID)
	dto, err := s.libraryDTO(r, lib)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

// checkLocalOverlap refuses local folders already covered by a library or
// nested within the request.
func (s *Server) checkLocalOverlap(w http.ResponseWriter, r *http.Request, inputs []sources.Input) bool {
	all, err := s.store.ListAllSources(r.Context())
	if err != nil {
		s.internalError(w, err)
		return false
	}
	var seen []string
	for _, in := range inputs {
		if in.Kind != sources.KindLocal {
			continue
		}
		clean := filepath.Clean(strings.TrimSpace(in.Root))
		for _, e := range all {
			if e.Kind == sources.KindLocal && overlaps(clean, e.Root) {
				writeError(w, http.StatusConflict, "path_overlap", "dossier déjà couvert par une bibliothèque : "+e.Root)
				return false
			}
		}
		for _, q := range seen {
			if overlaps(clean, q) {
				writeError(w, http.StatusBadRequest, "path_overlap", "dossiers imbriqués : "+q)
				return false
			}
		}
		seen = append(seen, clean)
	}
	return true
}

// sourceError writes validation errors as 400 and reports whether err was handled.
func (s *Server) sourceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var ve *sources.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusBadRequest, "invalid_source", ve.Msg)
		return true
	}
	if errors.Is(err, vfs.ErrOffline) {
		writeError(w, http.StatusBadGateway, "agent_offline", "agent injoignable à cette adresse")
		return true
	}
	s.internalError(w, err)
	return true
}

func (s *Server) enqueueScan(r *http.Request, libID int64) {
	if s.Scanner != nil {
		if _, err := s.Scanner.EnqueueScan(r.Context(), libID, false); err != nil {
			s.log.Error("enqueue scan", "err", err)
		}
	}
}

func (s *Server) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLibrary(w, r)
	if !ok {
		return
	}
	srcs, _ := s.store.ListSources(r.Context(), l.ID)
	if _, err := s.store.DeleteLibrary(r.Context(), l.ID); err != nil {
		s.internalError(w, err)
		return
	}
	if s.Sources != nil {
		for _, src := range srcs {
			s.Sources.Forget(src.ID)
		}
	}
	s.afterLibraryChange(r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) scanLibrary(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLibrary(w, r)
	if !ok {
		return
	}
	if s.Scanner == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "scanner indisponible")
		return
	}
	full := r.URL.Query().Get("full") == "true"
	queued, err := s.Scanner.EnqueueScan(r.Context(), l.ID, full)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": queued})
}

// testSource: POST /api/v1/sources/test
func (s *Server) testSource(w http.ResponseWriter, r *http.Request) {
	if !s.requireSources(w) {
		return
	}
	var in sources.Input
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	res, err := s.Sources.Test(r.Context(), in)
	if s.sourceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// addSource: POST /api/v1/libraries/{id}/sources
func (s *Server) addSource(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLibrary(w, r)
	if !ok || !s.requireSources(w) {
		return
	}
	var in sources.Input
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	if !s.checkLocalOverlap(w, r, []sources.Input{in}) {
		return
	}
	src, err := s.Sources.Create(r.Context(), s.store.Queries, l.ID, in)
	if s.sourceError(w, err) {
		return
	}
	s.afterLibraryChange(r)
	s.enqueueScan(r, l.ID)
	writeJSON(w, http.StatusCreated, toSourceDTO(src))
}

func (s *Server) loadSource(w http.ResponseWriter, r *http.Request) (db.LibrarySource, bool) {
	l, ok := s.loadLibrary(w, r)
	if !ok || !s.requireSources(w) {
		return db.LibrarySource{}, false
	}
	sid, err := strconv.ParseInt(r.PathValue("sid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "identifiant invalide")
		return db.LibrarySource{}, false
	}
	src, err := s.store.GetSource(r.Context(), sid)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && src.LibraryID != l.ID) {
		writeError(w, http.StatusNotFound, "not_found", "source introuvable")
		return src, false
	}
	if err != nil {
		s.internalError(w, err)
		return src, false
	}
	return src, true
}

// patchSource: PATCH /api/v1/libraries/{id}/sources/{sid}
func (s *Server) patchSource(w http.ResponseWriter, r *http.Request) {
	src, ok := s.loadSource(w, r)
	if !ok {
		return
	}
	// Start from the stored settings so partial updates work.
	var cfg sources.Config
	_ = json.Unmarshal([]byte(src.Config), &cfg)
	in := sources.Input{Kind: src.Kind, Name: src.Name, Root: src.Root}
	if cfg.Agent != nil {
		in.Agent = &sources.AgentIn{AgentID: cfg.Agent.AgentID, URL: cfg.Agent.URL, Share: cfg.Agent.Share}
	}
	if cfg.SFTP != nil {
		in.SFTP = &sources.SFTPInput{Host: cfg.SFTP.Host, Port: cfg.SFTP.Port, User: cfg.SFTP.User, HostKey: cfg.SFTP.HostKey}
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	updated, err := s.Sources.Update(r.Context(), src, in)
	if s.sourceError(w, err) {
		return
	}
	s.afterLibraryChange(r)
	s.enqueueScan(r, src.LibraryID)
	writeJSON(w, http.StatusOK, toSourceDTO(updated))
}

// deleteSource: DELETE /api/v1/libraries/{id}/sources/{sid}
func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	src, ok := s.loadSource(w, r)
	if !ok {
		return
	}
	if _, err := s.store.DeleteSource(r.Context(), db.DeleteSourceParams{ID: src.ID, LibraryID: src.LibraryID}); err != nil {
		s.internalError(w, err)
		return
	}
	s.Sources.Forget(src.ID)
	s.afterLibraryChange(r)
	s.enqueueScan(r, src.LibraryID) // flags items left without files as missing
	w.WriteHeader(http.StatusNoContent)
}

// vfsProxy: GET /internal/vfs/{id}/{exp}/{sig}/{path...} — serves files of
// remote sources to the server's own FFmpeg/ffprobe/image fetcher. Signed,
// expiring and restricted to loopback clients.
func (s *Server) vfsProxy(w http.ResponseWriter, r *http.Request) {
	if s.Sources == nil || !isLoopback(r.RemoteAddr) {
		http.NotFound(w, r)
		return
	}
	id, err1 := strconv.ParseInt(r.PathValue("id"), 10, 64)
	exp, err2 := strconv.ParseInt(r.PathValue("exp"), 10, 64)
	if err1 != nil || err2 != nil {
		http.NotFound(w, r)
		return
	}
	loc, ok := s.Sources.VerifyProxy(id, exp, r.PathValue("sig"), r.PathValue("path"))
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	f, err := s.Sources.Open(r.Context(), loc, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = f.Close() }()
	http.ServeContent(w, r, "", f.ModTime(), f)
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) afterLibraryChange(r *http.Request) {
	if s.Watcher != nil {
		if err := s.Watcher.Sync(r.Context()); err != nil {
			s.log.Error("watcher sync", "err", err)
		}
	}
}

func (s *Server) loadLibrary(w http.ResponseWriter, r *http.Request) (db.Library, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "identifiant invalide")
		return db.Library{}, false
	}
	l, err := s.store.GetLibrary(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "bibliothèque introuvable")
		return l, false
	}
	if err != nil {
		s.internalError(w, err)
		return l, false
	}
	return l, true
}

func overlaps(a, b string) bool {
	sep := string(filepath.Separator)
	return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, sep)+sep) || strings.HasPrefix(b, strings.TrimSuffix(a, sep)+sep)
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	s.log.Error("api", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "erreur interne")
}

// testStoredSource: POST /api/v1/libraries/{id}/sources/{sid}/test
func (s *Server) testStoredSource(w http.ResponseWriter, r *http.Request) {
	src, ok := s.loadSource(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.Sources.TestStored(r.Context(), src))
}
