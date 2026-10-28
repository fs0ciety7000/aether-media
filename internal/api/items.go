package api

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store"
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
	ChildCount        *int64                `json:"childCount,omitempty"`
	Available         bool                  `json:"available"`
	UserData          *userDataDTO          `json:"userData,omitempty"`
	UnplayedCount     *int64                `json:"unplayedCount,omitempty"`
	SeriesID          *int64                `json:"seriesId,omitempty"`
	SeriesTitle       string                `json:"seriesTitle,omitempty"`
	Video             *videoDTO             `json:"video,omitempty"`
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
	Total      int       `json:"total"`
}

type videoDTO struct {
	Resolution string `json:"resolution"`
	HDR        string `json:"hdr,omitempty"`
}

var sortKeys = map[string]bool{"title": true, "dateAdded": true, "year": true, "rating": true,
	"runtime": true, "premiereDate": true, "random": true, "lastPlayed": true}

// listItems: GET /api/v1/items — see docs/API.md for parameters.
func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := store.BrowseParams{Limit: 50, Genre: q.Get("genre"), Identify: q.Get("identify"),
		Resolution: q.Get("resolution"), HDR: q.Get("hdr") == "true", Sort: q.Get("sort")}
	ints := []struct {
		name string
		dst  *int64
	}{{"library", &p.LibraryID}, {"parent", &p.ParentID}, {"person", &p.PersonID}}
	for _, f := range ints {
		if v := q.Get(f.name); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_"+f.name, "paramètre "+f.name+" invalide")
				return
			}
			*f.dst = n
		}
	}
	for _, f := range []struct {
		name string
		dst  *int
	}{{"yearFrom", &p.YearFrom}, {"yearTo", &p.YearTo}, {"limit", &p.Limit}} {
		if v := q.Get(f.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_"+f.name, "paramètre "+f.name+" invalide")
				return
			}
			*f.dst = n
		}
	}
	if p.Limit < 1 || p.Limit > 200 {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit doit être entre 1 et 200")
		return
	}
	if k := q.Get("kind"); k != "" {
		p.Kinds = strings.Split(k, ",")
	}
	switch q.Get("played") {
	case "true":
		t := true
		p.Played = &t
	case "false":
		f := false
		p.Played = &f
	}
	p.InProgress = q.Get("inProgress") == "true"
	p.Favorite = q.Get("favorite") == "true"
	p.Watchlist = q.Get("watchlist") == "true"
	if p.Sort == "" {
		p.Sort = "title"
	}
	if !sortKeys[p.Sort] {
		writeError(w, http.StatusBadRequest, "invalid_sort", "tri inconnu")
		return
	}
	if p.Resolution != "" && store.Resolution(map[string]int64{"4K": 3840, "1080p": 1920, "720p": 1280, "SD": 720}[p.Resolution], 0) != p.Resolution {
		writeError(w, http.StatusBadRequest, "invalid_resolution", "résolution attendue : 4K, 1080p, 720p ou SD")
		return
	}
	switch q.Get("order") {
	case "asc":
	case "desc":
		p.Desc = true
	case "":
		p.Desc = p.Sort != "title"
	default:
		writeError(w, http.StatusBadRequest, "invalid_order", "order attendu : asc ou desc")
		return
	}
	if c := q.Get("cursor"); c != "" {
		off, seed, err := decodeCursor(c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "curseur invalide")
			return
		}
		p.Offset, p.Seed = off, seed
	} else if p.Sort == "random" {
		p.Seed = rand.Int64N(1 << 31)
	}

	rows, total, err := s.browse(r, p)
	if err != nil {
		s.internalError(w, err)
		return
	}
	items, err := s.itemDTOs(r, rows)
	if err != nil {
		s.internalError(w, err)
		return
	}
	page := itemPage{Items: items, Total: total}
	if next := p.Offset + len(rows); len(rows) == p.Limit && next < total {
		page.NextCursor = encodeCursor(next, p.Seed)
	}
	writeJSON(w, http.StatusOK, page)
}

// browse lists items as seen by the requesting user.
func (s *Server) browse(r *http.Request, p store.BrowseParams) ([]store.BrowseRow, int, error) {
	p.Viewer = viewer(r)
	return s.store.Browse(r.Context(), p)
}

// visible reports whether the requesting user may see an item.
func (s *Server) visible(r *http.Request, itemID int64) bool {
	rows, _, err := s.browse(r, store.BrowseParams{IDs: []int64{itemID}, Limit: 1})
	return err == nil && len(rows) == 1
}

type userDataDTO struct {
	PositionMs   int64      `json:"positionMs"`
	Played       bool       `json:"played"`
	PlayCount    int64      `json:"playCount"`
	LastPlayedAt *time.Time `json:"lastPlayedAt,omitempty"`
	Favorite     bool       `json:"favorite"`
	InWatchlist  bool       `json:"inWatchlist"`
	Rating       *float64   `json:"rating,omitempty"`
}

func userDataOf(row store.BrowseRow) *userDataDTO {
	ud := &userDataDTO{PositionMs: row.PositionMs, Played: row.Played, PlayCount: row.PlayCount, Favorite: row.Favorite, InWatchlist: row.InWatchlist}
	if row.LastPlayedAt.Valid {
		t := time.Unix(row.LastPlayedAt.Int64, 0).UTC()
		ud.LastPlayedAt = &t
	}
	if row.UserRating.Valid {
		v := row.UserRating.Float64
		ud.Rating = &v
	}
	return ud
}

// itemDTOs converts browse rows, loading their images in one query.
func (s *Server) itemDTOs(r *http.Request, rows []store.BrowseRow) ([]itemDTO, error) {
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	imgs, err := s.store.ImagesFor(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	admin := isAdmin(r)
	out := make([]itemDTO, 0, len(rows))
	for _, row := range rows {
		dto := toItemDTO(row.Item)
		dto.Images = imageDTOs(row.ID, imgs[row.ID])
		cc := row.ChildCount
		dto.ChildCount = &cc
		dto.Available = row.Available
		dto.UserData = userDataOf(row)
		if row.UnplayedCount.Valid {
			n := row.UnplayedCount.Int64
			dto.UnplayedCount = &n
		}
		if row.SeriesID.Valid {
			id := row.SeriesID.Int64
			dto.SeriesID, dto.SeriesTitle = &id, row.SeriesTitle.String
		}
		if !admin {
			dto.Path = ""
		}
		if res := store.Resolution(row.VideoWidth, row.VideoHeight); res != "" {
			dto.Video = &videoDTO{Resolution: res, HDR: row.HDR}
		}
		out = append(out, dto)
	}
	return out, nil
}

// browseIDs loads items by id, preserving the given order.
func (s *Server) browseIDs(r *http.Request, ids []int64) ([]itemDTO, error) {
	if len(ids) == 0 {
		return []itemDTO{}, nil
	}
	rows, _, err := s.browse(r, store.BrowseParams{IDs: ids, Limit: len(ids)})
	if err != nil {
		return nil, err
	}
	items, err := s.itemDTOs(r, rows)
	if err != nil {
		return nil, err
	}
	pos := make(map[int64]int, len(ids))
	for i, id := range ids {
		pos[id] = i
	}
	sort.SliceStable(items, func(a, b int) bool { return pos[items[a].ID] < pos[items[b].ID] })
	return items, nil
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

type ancestorDTO struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

type itemDetail struct {
	itemDTO
	Ancestors  []ancestorDTO  `json:"ancestors"`
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
	rows, _, err := s.browse(r, store.BrowseParams{IDs: []int64{id}, Limit: 1})
	if err != nil {
		s.internalError(w, err)
		return
	}
	if len(rows) == 0 { // hidden from this user (library access, parental control)
		writeError(w, http.StatusNotFound, "not_found", "élément introuvable")
		return
	}
	dtos, err := s.itemDTOs(r, rows)
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := itemDetail{itemDTO: dtos[0], Ancestors: []ancestorDTO{}, MediaFiles: []mediaFileDTO{}, Genres: []string{}, Studios: []string{}, People: []personDTO{}}
	for pid := it.ParentID; pid.Valid; {
		parent, err := s.store.GetItem(ctx, pid.Int64)
		if err != nil {
			break
		}
		out.Ancestors = append([]ancestorDTO{{ID: parent.ID, Kind: parent.Kind, Title: parent.Title}}, out.Ancestors...)
		pid = parent.ParentID
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
		out.People = append(out.People, personDTO{ID: p.ID, Name: p.Name, Kind: p.Kind, Role: p.Role, Image: personImageURL(p.ID, p.ImageUrl)})
	}
	files, err := s.store.ListItemMediaFiles(ctx, id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	for _, f := range files {
		path := f.Path
		if !isAdmin(r) {
			path = filepath.Base(f.Path)
		}
		dto := mediaFileDTO{ID: f.ID, Path: path, Size: f.Size, Part: f.Part, Container: f.Container.String,
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

func encodeCursor(offset int, seed int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("o:%d:%d", offset, seed)))
}

func decodeCursor(c string) (int, int64, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, 0, err
	}
	var off int
	var seed int64
	if _, err := fmt.Sscanf(string(b), "o:%d:%d", &off, &seed); err != nil || off < 0 {
		return 0, 0, errors.New("malformed cursor")
	}
	return off, seed, nil
}

func storeIDs(ids ...int64) store.BrowseParams { return store.BrowseParams{IDs: ids, Limit: len(ids)} }
