package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/library"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

type libraryDTO struct {
	ID        int64            `json:"id"`
	Name      string           `json:"name"`
	Kind      string           `json:"kind"`
	Paths     []string         `json:"paths"`
	Counts    map[string]int64 `json:"counts"`
	CreatedAt time.Time        `json:"createdAt"`
}

func (s *Server) libraryDTO(r *http.Request, l db.Library) (libraryDTO, error) {
	dto := libraryDTO{ID: l.ID, Name: l.Name, Kind: l.Kind, Paths: []string{}, Counts: map[string]int64{},
		CreatedAt: time.Unix(l.CreatedAt, 0).UTC()}
	paths, err := s.store.ListLibraryPaths(r.Context(), l.ID)
	if err != nil {
		return dto, err
	}
	for _, p := range paths {
		dto.Paths = append(dto.Paths, p.Path)
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

func (s *Server) listLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.store.ListLibraries(r.Context())
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
	dto, err := s.libraryDTO(r, l)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

type createLibraryRequest struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	Paths []string `json:"paths"`
}

var libraryKinds = map[string]bool{
	library.KindMovies: true, library.KindShows: true, library.KindMusic: true, library.KindMixed: true,
}

func (s *Server) createLibrary(w http.ResponseWriter, r *http.Request) {
	var req createLibraryRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
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
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_paths", "au moins un dossier est requis")
		return
	}
	existing, err := s.store.ListAllLibraryPaths(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	paths := make([]string, 0, len(req.Paths))
	for _, p := range req.Paths {
		clean := filepath.Clean(p)
		if !filepath.IsAbs(clean) {
			writeError(w, http.StatusBadRequest, "invalid_paths", "chemin absolu requis : "+p)
			return
		}
		if st, err := os.Stat(clean); err != nil || !st.IsDir() {
			writeError(w, http.StatusBadRequest, "invalid_paths", "dossier introuvable : "+p)
			return
		}
		for _, e := range existing {
			if overlaps(clean, e.Path) {
				writeError(w, http.StatusConflict, "path_overlap", "dossier déjà couvert par une bibliothèque : "+e.Path)
				return
			}
		}
		for _, q := range paths {
			if overlaps(clean, q) {
				writeError(w, http.StatusBadRequest, "path_overlap", "dossiers imbriqués : "+q)
				return
			}
		}
		paths = append(paths, clean)
	}

	var lib db.Library
	err = s.store.Tx(r.Context(), func(q *db.Queries) error {
		var err error
		if lib, err = q.CreateLibrary(r.Context(), db.CreateLibraryParams{Name: req.Name, Kind: req.Kind}); err != nil {
			return err
		}
		for _, p := range paths {
			if err := q.AddLibraryPath(r.Context(), db.AddLibraryPathParams{LibraryID: lib.ID, Path: p}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.afterLibraryChange(r)
	if s.Scanner != nil {
		if _, err := s.Scanner.EnqueueScan(r.Context(), lib.ID, false); err != nil {
			s.log.Error("enqueue scan", "err", err)
		}
	}
	dto, err := s.libraryDTO(r, lib)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

func (s *Server) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLibrary(w, r)
	if !ok {
		return
	}
	if _, err := s.store.DeleteLibrary(r.Context(), l.ID); err != nil {
		s.internalError(w, err)
		return
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
