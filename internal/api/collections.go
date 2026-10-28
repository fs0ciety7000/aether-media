package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

type collectionDTO struct {
	ID      string                `json:"id"` // "c<id>" manual, "t<name>" automatic (TMDB)
	Name    string                `json:"name"`
	Kind    string                `json:"kind"`
	Count   int                   `json:"count"`
	Images  map[string][]imageDTO `json:"images"`
	OwnerID *int64                `json:"ownerId,omitempty"`
	Shared  bool                  `json:"shared"`
}

// collectionImages uses the first visible item's artwork.
func (s *Server) collectionImages(r *http.Request, ids []int64) (map[string][]imageDTO, int) {
	items, err := s.browseIDs(r, ids)
	if err != nil || len(items) == 0 {
		return map[string][]imageDTO{}, 0
	}
	return items[0].Images, len(items)
}

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := principalFrom(r).user.ID
	out := []collectionDTO{}
	manual, err := s.store.ListCollectionsForUser(ctx, sql.NullInt64{Int64: uid, Valid: true})
	if err != nil {
		s.internalError(w, err)
		return
	}
	for _, c := range manual {
		ids, err := s.store.ListCollectionItemIDs(ctx, c.ID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		imgs, n := s.collectionImages(r, ids)
		dto := collectionDTO{ID: "c" + strconv.FormatInt(c.ID, 10), Name: c.Name, Kind: "manual", Count: n, Images: imgs, Shared: c.Shared == 1}
		if c.OwnerID.Valid {
			o := c.OwnerID.Int64
			dto.OwnerID = &o
		}
		out = append(out, dto)
	}
	auto, err := s.store.ListAutoCollections(ctx)
	if err != nil {
		s.internalError(w, err)
		return
	}
	for _, c := range auto {
		name := c.Collection.String
		ids, err := s.store.ListAutoCollectionItemIDs(ctx, c.Collection)
		if err != nil {
			s.internalError(w, err)
			return
		}
		imgs, n := s.collectionImages(r, ids)
		if n < 2 { // hidden items may leave less than a collection
			continue
		}
		out = append(out, collectionDTO{ID: "t" + name, Name: name, Kind: "tmdb", Count: n, Images: imgs, Shared: true})
	}
	writeJSON(w, http.StatusOK, out)
}

// loadCollection resolves a manual collection the user may read (and, with
// write, modify: owner, or admin for shared ones).
func (s *Server) loadCollection(w http.ResponseWriter, r *http.Request, write bool) (db.Collection, bool) {
	cid := r.PathValue("cid")
	id, err := strconv.ParseInt(strings.TrimPrefix(cid, "c"), 10, 64)
	if !strings.HasPrefix(cid, "c") || err != nil {
		if write {
			writeError(w, http.StatusBadRequest, "read_only", "les collections automatiques ne sont pas modifiables")
		} else {
			writeError(w, http.StatusNotFound, "not_found", "collection introuvable")
		}
		return db.Collection{}, false
	}
	c, err := s.store.GetCollection(r.Context(), id)
	p := principalFrom(r)
	owner := c.OwnerID.Valid && c.OwnerID.Int64 == p.user.ID
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !owner && c.Shared == 0) {
		writeError(w, http.StatusNotFound, "not_found", "collection introuvable")
		return c, false
	}
	if err != nil {
		s.internalError(w, err)
		return c, false
	}
	if write && !owner && !isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "collection en lecture seule")
		return c, false
	}
	return c, true
}

func (s *Server) collectionItems(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("cid")
	var ids []int64
	var dto collectionDTO
	var err error
	if name, ok := strings.CutPrefix(cid, "t"); ok {
		ids, err = s.store.ListAutoCollectionItemIDs(r.Context(), sql.NullString{String: name, Valid: true})
		dto = collectionDTO{ID: cid, Name: name, Kind: "tmdb", Shared: true}
	} else {
		c, ok := s.loadCollection(w, r, false)
		if !ok {
			return
		}
		ids, err = s.store.ListCollectionItemIDs(r.Context(), c.ID)
		dto = collectionDTO{ID: cid, Name: c.Name, Kind: "manual", Shared: c.Shared == 1}
		if c.OwnerID.Valid {
			o := c.OwnerID.Int64
			dto.OwnerID = &o
		}
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	items, err := s.browseIDs(r, ids)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if dto.Kind == "tmdb" && len(items) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "collection introuvable")
		return
	}
	dto.Count = len(items)
	dto.Images = map[string][]imageDTO{}
	if len(items) > 0 {
		dto.Images = items[0].Images
	}
	writeJSON(w, http.StatusOK, map[string]any{"collection": dto, "items": items})
}

type collectionRequest struct {
	Name   *string `json:"name"`
	Shared *bool   `json:"shared"`
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) {
	var req collectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil || req.Name == nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "nom requis")
		return
	}
	name := strings.TrimSpace(*req.Name)
	if name == "" || len(name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "nom requis (100 caractères max)")
		return
	}
	shared := req.Shared != nil && *req.Shared
	if shared && !isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "seul un administrateur peut partager une collection")
		return
	}
	uid := principalFrom(r).user.ID
	c, err := s.store.CreateCollection(r.Context(), db.CreateCollectionParams{OwnerID: sql.NullInt64{Int64: uid, Valid: true}, Name: name, Shared: b2i(shared)})
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, collectionDTO{ID: "c" + strconv.FormatInt(c.ID, 10), Name: c.Name, Kind: "manual",
		Images: map[string][]imageDTO{}, OwnerID: &uid, Shared: shared})
}

func (s *Server) patchCollection(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadCollection(w, r, true)
	if !ok {
		return
	}
	var req collectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	name, shared := c.Name, c.Shared == 1
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_name", "nom requis (100 caractères max)")
			return
		}
	}
	if req.Shared != nil {
		if !isAdmin(r) {
			writeError(w, http.StatusForbidden, "forbidden", "seul un administrateur peut partager une collection")
			return
		}
		shared = *req.Shared
	}
	if err := s.store.UpdateCollection(r.Context(), db.UpdateCollectionParams{Name: name, Shared: b2i(shared), ID: c.ID}); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteCollection(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadCollection(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeleteCollection(r.Context(), c.ID); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) collectionMember(w http.ResponseWriter, r *http.Request) (db.Collection, int64, bool) {
	c, ok := s.loadCollection(w, r, true)
	if !ok {
		return c, 0, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !s.visible(r, id) {
		writeError(w, http.StatusNotFound, "not_found", "élément introuvable")
		return c, 0, false
	}
	return c, id, true
}

func (s *Server) addToCollection(w http.ResponseWriter, r *http.Request) {
	c, id, ok := s.collectionMember(w, r)
	if !ok {
		return
	}
	if err := s.store.AddCollectionItem(r.Context(), db.AddCollectionItemParams{CollectionID: c.ID, ItemID: id}); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeFromCollection(w http.ResponseWriter, r *http.Request) {
	c, id, ok := s.collectionMember(w, r)
	if !ok {
		return
	}
	if err := s.store.RemoveCollectionItem(r.Context(), db.RemoveCollectionItemParams{CollectionID: c.ID, ItemID: id}); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
