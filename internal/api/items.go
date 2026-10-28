package api

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// itemDTO is the public shape of an item. Paths are exposed for now; they
// will be restricted to administrators with authentication (M6).
type itemDTO struct {
	ID                int64                 `json:"id"`
	LibraryID         int64                 `json:"libraryId"`
	ParentID          *int64                `json:"parentId,omitempty"`
	Kind              string                `json:"kind"`
	Title             string                `json:"title"`
	OriginalTitle     string                `json:"originalTitle,omitempty"`
	Year              *int64                `json:"year,omitempty"`
	Overview          string                `json:"overview,omitempty"`
	RuntimeMs         *int64                `json:"runtimeMs,omitempty"`
	IndexNumber       *int64                `json:"indexNumber,omitempty"`
	IndexNumberEnd    *int64                `json:"indexNumberEnd,omitempty"`
	ParentIndexNumber *int64                `json:"parentIndexNumber,omitempty"`
	Edition           string                `json:"edition,omitempty"`
	AirDate           string                `json:"airDate,omitempty"`
	ProviderIDs       map[string]string     `json:"providerIds"`
	Path              string                `json:"path,omitempty"`
	DateAdded         time.Time             `json:"dateAdded"`
	Tagline           string                `json:"tagline,omitempty"`
	PremiereDate      string                `json:"premiereDate,omitempty"`
	EndDate           string                `json:"endDate,omitempty"`
	Status            string                `json:"status,omitempty"`
	OfficialRating    string                `json:"officialRating,omitempty"`
	CommunityRating   *float64              `json:"communityRating,omitempty"`
	Collection        string                `json:"collection,omitempty"`
	IdentifyStatus    string                `json:"identifyStatus"`
	LockedFields      []string              `json:"lockedFields"`
	Images            map[string][]imageDTO `json:"images"`
}

func toItemDTO(it db.Item) itemDTO {
	ids := map[string]string{}
	_ = json.Unmarshal([]byte(it.ProviderIds), &ids)
	return itemDTO{
		ID: it.ID, LibraryID: it.LibraryID, ParentID: ptr(it.ParentID), Kind: it.Kind, Title: it.Title,
		OriginalTitle: it.OriginalTitle.String, Year: ptr(it.Year), Overview: it.Overview.String,
		RuntimeMs: ptr(it.RuntimeMs), IndexNumber: ptr(it.IndexNumber), IndexNumberEnd: ptr(it.IndexNumberEnd),
		ParentIndexNumber: ptr(it.ParentIndexNumber), Edition: it.Edition.String, AirDate: it.AirDate.String,
		ProviderIDs: ids, Path: it.Path.String, DateAdded: time.Unix(it.DateAdded, 0).UTC(),
		Tagline: it.Tagline.String, PremiereDate: it.PremiereDate.String, EndDate: it.EndDate.String,
		Status: it.Status.String, OfficialRating: it.OfficialRating.String, Collection: it.Collection.String,
		IdentifyStatus: it.IdentifyStatus, LockedFields: locked(it.LockedFields), Images: map[string][]imageDTO{},
		CommunityRating: func() *float64 {
			if !it.CommunityRating.Valid {
				return nil
			}
			return &it.CommunityRating.Float64
		}(),
	}
}

func locked(s string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

type itemPage struct {
	Items      []itemDTO `json:"items"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

// listItems: GET /api/v1/items?library=&kind=&parent=&limit=&cursor=
// Items are ordered by sort title; the cursor is opaque.
func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := db.ListItemsParams{Lim: 50}
	var err error
	if p.LibraryID, err = optInt(q.Get("library")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_library", "paramètre library invalide")
		return
	}
	if p.ParentID, err = optInt(q.Get("parent")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_parent", "paramètre parent invalide")
		return
	}
	if k := q.Get("kind"); k != "" {
		p.Kind = sql.NullString{String: k, Valid: true}
	}
	if st := q.Get("identify"); st != "" {
		p.IdentifyStatus = sql.NullString{String: st, Valid: true}
	}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit doit être entre 1 et 500")
			return
		}
		p.Lim = int64(n)
	}
	if c := q.Get("cursor"); c != "" {
		if p.AfterSortTitle, p.AfterID, err = decodeCursor(c); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "curseur invalide")
			return
		}
	}
	items, err := s.store.ListItems(r.Context(), p)
	if err != nil {
		s.internalError(w, err)
		return
	}
	page := itemPage{Items: make([]itemDTO, 0, len(items))}
	for _, it := range items {
		dto := toItemDTO(it)
		if dto.Images, err = s.imagesFor(r, it.ID); err != nil {
			s.internalError(w, err)
			return
		}
		page.Items = append(page.Items, dto)
	}
	if int64(len(items)) == p.Lim {
		last := items[len(items)-1]
		page.NextCursor = encodeCursor(last.SortTitle, last.ID)
	}
	writeJSON(w, http.StatusOK, page)
}

type mediaStreamDTO struct {
	Index         int64   `json:"index"`
	Type          string  `json:"type"`
	Codec         string  `json:"codec"`
	Profile       string  `json:"profile,omitempty"`
	Width         int64   `json:"width,omitempty"`
	Height        int64   `json:"height,omitempty"`
	FrameRate     float64 `json:"frameRate,omitempty"`
	BitDepth      int64   `json:"bitDepth,omitempty"`
	HDR           string  `json:"hdr,omitempty"`
	Channels      int64   `json:"channels,omitempty"`
	ChannelLayout string  `json:"channelLayout,omitempty"`
	Language      string  `json:"language,omitempty"`
	Title         string  `json:"title,omitempty"`
	Default       bool    `json:"default"`
	Forced        bool    `json:"forced"`
	ImageSub      bool    `json:"imageSub,omitempty"`
}

type mediaFileDTO struct {
	ID         int64            `json:"id"`
	Path       string           `json:"path"`
	Size       int64            `json:"size"`
	Part       int64            `json:"part,omitempty"`
	Container  string           `json:"container,omitempty"`
	DurationMs *int64           `json:"durationMs,omitempty"`
	Bitrate    *int64           `json:"bitrate,omitempty"`
	Probed     bool             `json:"probed"`
	ProbeError string           `json:"probeError,omitempty"`
	Streams    []mediaStreamDTO `json:"streams"`
}

type itemDetail struct {
	itemDTO
	ChildCount int64          `json:"childCount"`
	Genres     []string       `json:"genres"`
	Studios    []string       `json:"studios"`
	People     []personDTO    `json:"people"`
	MediaFiles []mediaFileDTO `json:"mediaFiles"`
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "identifiant invalide")
		return
	}
	ctx := r.Context()
	it, err := s.store.GetItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "élément introuvable")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := itemDetail{itemDTO: toItemDTO(it), MediaFiles: []mediaFileDTO{}, Genres: []string{}, Studios: []string{}, People: []personDTO{}}
	if out.Images, err = s.imagesFor(r, id); err != nil {
		s.internalError(w, err)
		return
	}
	if g, err := s.store.ListItemGenres(ctx, id); err == nil && g != nil {
		out.Genres = g
	}
	if st, err := s.store.ListItemStudios(ctx, id); err == nil && st != nil {
		out.Studios = st
	}
	people, err := s.store.ListItemPeople(ctx, id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	for _, p := range people {
		out.People = append(out.People, personDTO{ID: p.ID, Name: p.Name, Kind: p.Kind, Role: p.Role, Image: p.ImageUrl})
	}
	if out.ChildCount, err = s.store.CountChildren(ctx, sql.NullInt64{Int64: id, Valid: true}); err != nil {
		s.internalError(w, err)
		return
	}
	files, err := s.store.ListItemMediaFiles(ctx, id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	for _, f := range files {
		dto := mediaFileDTO{ID: f.ID, Path: f.Path, Size: f.Size, Part: f.Part, Container: f.Container.String,
			DurationMs: ptr(f.DurationMs), Bitrate: ptr(f.Bitrate), Probed: f.ProbedAt.Valid,
			ProbeError: f.ProbeError.String, Streams: []mediaStreamDTO{}}
		streams, err := s.store.ListMediaStreams(ctx, f.ID)
		if err != nil {
			s.internalError(w, err)
			return
		}
		for _, st := range streams {
			dto.Streams = append(dto.Streams, mediaStreamDTO{
				Index: st.Idx, Type: st.Type, Codec: st.Codec, Profile: st.Profile, Width: st.Width, Height: st.Height,
				FrameRate: st.FrameRate, BitDepth: st.BitDepth, HDR: st.Hdr, Channels: st.Channels,
				ChannelLayout: st.ChannelLayout, Language: st.Language, Title: st.Title,
				Default: st.IsDefault == 1, Forced: st.IsForced == 1, ImageSub: st.IsImageSub == 1,
			})
		}
		out.MediaFiles = append(out.MediaFiles, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) jobStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.JobStats(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := map[string]map[string]int64{}
	for _, st := range stats {
		if out[st.Kind] == nil {
			out[st.Kind] = map[string]int64{}
		}
		out[st.Kind][st.Status] = st.N
	}
	writeJSON(w, http.StatusOK, out)
}

func optInt(v string) (sql.NullInt64, error) {
	if v == "" {
		return sql.NullInt64{}, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return sql.NullInt64{Int64: n, Valid: err == nil}, err
}

func encodeCursor(sortTitle string, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%s", id, sortTitle)))
}

func decodeCursor(c string) (string, int64, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", 0, err
	}
	idStr, title, ok := strings.Cut(string(b), ":")
	if !ok {
		return "", 0, errors.New("malformed cursor")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	return title, id, err
}
