package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/naming"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

type imageDTO struct {
	URL      string `json:"url"` // versioned: safe to cache forever
	Width    int64  `json:"width,omitempty"`
	Height   int64  `json:"height,omitempty"`
	Blurhash string `json:"blurhash,omitempty"`
	Dominant string `json:"dominant,omitempty"`
}

type personDTO struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Role  string `json:"role,omitempty"`
	Image string `json:"image,omitempty"` // remote URL for now
}

// imageDTOs groups fetched images by type ("backdrop" may hold several).
func imageDTOs(itemID int64, rows []db.Image) map[string][]imageDTO {
	out := map[string][]imageDTO{}
	for _, img := range rows {
		if !img.Hash.Valid {
			continue
		}
		out[img.Type] = append(out[img.Type], imageDTOFor(itemID, img))
	}
	return out
}

func imageDTOFor(itemID int64, img db.Image) imageDTO {
	u := "/api/v1/items/" + strconv.FormatInt(itemID, 10) + "/images/" + img.Type
	q := "v=" + img.Hash.String[:12]
	if img.Idx > 0 {
		q = "idx=" + strconv.FormatInt(img.Idx, 10) + "&" + q
	}
	return imageDTO{URL: u + "?" + q, Width: img.Width, Height: img.Height, Blurhash: img.Blurhash, Dominant: img.Dominant}
}

// itemImage: GET /api/v1/items/{id}/images/{type}?w=342&idx=0
func (s *Server) itemImage(w http.ResponseWriter, r *http.Request) {
	if s.Images == nil {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	idx, _ := strconv.ParseInt(r.URL.Query().Get("idx"), 10, 64)
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))
	img, err := s.store.GetItemImage(r.Context(), db.GetItemImageParams{ItemID: id, Type: r.PathValue("type"), Idx: idx})
	if err != nil || !img.Hash.Valid {
		http.NotFound(w, r)
		return
	}
	s.Images.ServeVariant(w, r, img.Hash.String, width)
}

type patchItemRequest struct {
	Title          *string `json:"title"`
	OriginalTitle  *string `json:"originalTitle"`
	Overview       *string `json:"overview"`
	Tagline        *string `json:"tagline"`
	Year           *int64  `json:"year"`
	OfficialRating *string `json:"officialRating"`
	// Unlock lists fields to release back to automatic updates.
	Unlock []string `json:"unlock"`
}

// patchItem edits fields by hand; edited fields are locked against refreshes.
func (s *Server) patchItem(w http.ResponseWriter, r *http.Request) {
	it, ok := s.loadItem(w, r)
	if !ok {
		return
	}
	var req patchItemRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	var locked []string
	_ = json.Unmarshal([]byte(it.LockedFields), &locked)
	lock := func(f string) {
		if !slices.Contains(locked, f) {
			locked = append(locked, f)
		}
	}
	p := db.UpdateItemFieldsParams{ID: it.ID}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" {
			writeError(w, http.StatusBadRequest, "invalid_title", "le titre ne peut pas être vide")
			return
		}
		p.Title = sql.NullString{String: t, Valid: true}
		if it.Kind == metadata.KindMovie || it.Kind == metadata.KindSeries || it.Kind == metadata.KindAlbum || it.Kind == metadata.KindArtist {
			p.SortTitle = sql.NullString{String: naming.SortTitle(t), Valid: true}
		}
		lock("title")
	}
	setStr := func(dst *sql.NullString, v *string, field string) {
		if v != nil {
			*dst = sql.NullString{String: strings.TrimSpace(*v), Valid: true}
			lock(field)
		}
	}
	setStr(&p.OriginalTitle, req.OriginalTitle, "originalTitle")
	setStr(&p.Overview, req.Overview, "overview")
	setStr(&p.Tagline, req.Tagline, "tagline")
	setStr(&p.OfficialRating, req.OfficialRating, "officialRating")
	if req.Year != nil {
		if *req.Year < 1800 || *req.Year > 2200 {
			writeError(w, http.StatusBadRequest, "invalid_year", "année invalide")
			return
		}
		p.Year = sql.NullInt64{Int64: *req.Year, Valid: true}
		lock("year")
	}
	locked = slices.DeleteFunc(locked, func(f string) bool { return slices.Contains(req.Unlock, f) })
	b, _ := json.Marshal(locked)
	if locked == nil {
		b = []byte("[]")
	}
	p.LockedFields = string(b)
	if err := s.store.UpdateItemFields(r.Context(), p); err != nil {
		s.internalError(w, err)
		return
	}
	r.SetPathValue("id", strconv.FormatInt(it.ID, 10))
	s.getItem(w, r)
}

func (s *Server) refreshItem(w http.ResponseWriter, r *http.Request) {
	it, ok := s.loadItem(w, r)
	if !ok || !s.requireRefresher(w) {
		return
	}
	queued, err := s.Refresher.Enqueue(r.Context(), it.ID, r.URL.Query().Get("research") == "true")
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": queued})
}

type identifySearchRequest struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
}

func (s *Server) identifySearch(w http.ResponseWriter, r *http.Request) {
	it, ok := s.loadItem(w, r)
	if !ok || !s.requireRefresher(w) {
		return
	}
	var req identifySearchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	cands, err := s.Refresher.Search(r.Context(), it.ID, strings.TrimSpace(req.Title), req.Year)
	if errors.Is(err, metadata.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "provider_unavailable", "fournisseur de métadonnées indisponible (clé API ?)")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	if cands == nil {
		cands = []metadata.Candidate{}
	}
	writeJSON(w, http.StatusOK, cands)
}

type identifyRequest struct {
	ProviderIDs metadata.ProviderIDs `json:"providerIds"`
}

func (s *Server) identify(w http.ResponseWriter, r *http.Request) {
	it, ok := s.loadItem(w, r)
	if !ok || !s.requireRefresher(w) {
		return
	}
	var req identifyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil || len(req.ProviderIDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_ids", "providerIds requis")
		return
	}
	for k, v := range req.ProviderIDs {
		if k == "" || v == "" || len(k) > 40 || len(v) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_ids", "identifiant invalide")
			return
		}
	}
	if err := s.Refresher.Identify(r.Context(), it.ID, req.ProviderIDs); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) requireRefresher(w http.ResponseWriter) bool {
	if s.Refresher == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "métadonnées indisponibles")
		return false
	}
	return true
}

func (s *Server) loadItem(w http.ResponseWriter, r *http.Request) (db.Item, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "identifiant invalide")
		return db.Item{}, false
	}
	it, err := s.store.GetItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "élément introuvable")
		return it, false
	}
	if err != nil {
		s.internalError(w, err)
		return it, false
	}
	return it, true
}
