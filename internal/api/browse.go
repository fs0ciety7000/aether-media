package api

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/fs0ciety7000/aether-media/internal/store"
)

type filtersDTO struct {
	Genres      []store.GenreCount `json:"genres"`
	Years       *yearRange         `json:"years"`
	Resolutions []string           `json:"resolutions"`
	HasHDR      bool               `json:"hasHdr"`
}

type yearRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func (s *Server) libraryFilters(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLibrary(w, r)
	if !ok {
		return
	}
	f, err := s.store.Filters(r.Context(), l.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := filtersDTO{Genres: f.Genres, Resolutions: f.Resolutions, HasHDR: f.HasHDR}
	if out.Genres == nil {
		out.Genres = []store.GenreCount{}
	}
	if out.Resolutions == nil {
		out.Resolutions = []string{}
	}
	if f.YearMin > 0 {
		out.Years = &yearRange{Min: f.YearMin, Max: f.YearMax}
	}
	writeJSON(w, http.StatusOK, out)
}

type libraryRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type recentRow struct {
	Library libraryRef `json:"library"`
	Items   []itemDTO  `json:"items"`
}

type categoryDTO struct {
	Genre string    `json:"genre"`
	Count int       `json:"count"`
	Image *imageDTO `json:"image,omitempty"`
}

type homeDTO struct {
	Hero             []itemDTO     `json:"hero"`
	ContinueWatching []itemDTO     `json:"continueWatching"`
	RecentlyAdded    []recentRow   `json:"recentlyAdded"`
	Categories       []categoryDTO `json:"categories"`
	Recommended      []itemDTO     `json:"recommended"`
}

// home assembles the landing page in one request.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := homeDTO{ContinueWatching: []itemDTO{}, RecentlyAdded: []recentRow{}, Categories: []categoryDTO{}}
	browse := func(p store.BrowseParams) ([]itemDTO, error) {
		rows, _, err := s.store.Browse(ctx, p)
		if err != nil {
			return nil, err
		}
		return s.itemDTOs(r, rows)
	}
	var err error
	if out.Hero, err = browse(store.BrowseParams{Kinds: []string{"movie", "series"}, HasImage: "backdrop", Sort: "dateAdded", Desc: true, Limit: 5}); err != nil {
		s.internalError(w, err)
		return
	}

	libs, err := s.store.ListLibraries(ctx)
	if err != nil {
		s.internalError(w, err)
		return
	}
	for _, l := range libs {
		p := store.BrowseParams{LibraryID: l.ID, Sort: "dateAdded", Desc: true, Limit: 16}
		switch l.Kind {
		case "shows":
			p.Kinds, p.Sort = []string{"series"}, "latest"
		case "music":
			p.Kinds = []string{"album"}
		case "mixed":
			p.Kinds = []string{"movie", "series", "album"}
		}
		items, err := browse(p)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if len(items) > 0 {
			out.RecentlyAdded = append(out.RecentlyAdded, recentRow{Library: libraryRef{ID: l.ID, Name: l.Name, Kind: l.Kind}, Items: items})
		}
	}

	genres, err := s.store.TopGenres(ctx, 8)
	if err != nil {
		s.internalError(w, err)
		return
	}
	seed := rand.Int64N(1 << 31)
	for _, g := range genres {
		c := categoryDTO{Genre: g.Name, Count: g.Count}
		if img, ok, err := s.store.GenreImage(ctx, g.Name, seed); err != nil {
			s.internalError(w, err)
			return
		} else if ok {
			dto := imageDTOFor(img.ItemID, img)
			c.Image = &dto
		}
		out.Categories = append(out.Categories, c)
	}

	if out.Recommended, err = browse(store.BrowseParams{Kinds: []string{"movie", "series"}, HasImage: "poster", Sort: "random", Seed: seed, Limit: 16}); err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type personHitDTO struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
	Count int    `json:"count"`
}

type searchDTO struct {
	Query   string         `json:"query"`
	Results map[string]any `json:"results"`
}

var searchKinds = []string{"movie", "series", "episode", "artist", "album", "track"}

// search: GET /api/v1/search?q=&limit=8 — grouped by kind.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 8
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 50 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit doit être entre 1 et 50")
			return
		}
		limit = n
	}
	out := searchDTO{Query: q, Results: map[string]any{}}
	grouped := map[string][]int64{}
	people := []personHitDTO{}
	if utf8.RuneCountInString(q) >= 2 && len(q) <= 200 {
		hits, err := s.store.SearchItems(r.Context(), q, 300)
		if err != nil {
			s.internalError(w, err)
			return
		}
		for _, it := range hits {
			if len(grouped[it.Kind]) < limit {
				grouped[it.Kind] = append(grouped[it.Kind], it.ID)
			}
		}
		ph, err := s.store.SearchPeople(r.Context(), q, limit)
		if err != nil {
			s.internalError(w, err)
			return
		}
		for _, p := range ph {
			people = append(people, personHitDTO{ID: p.ID, Name: p.Name, Image: p.ImageURL, Count: p.Count})
		}
	}
	for _, k := range searchKinds {
		items, err := s.browseIDs(r, grouped[k])
		if err != nil {
			s.internalError(w, err)
			return
		}
		out.Results[k] = items
	}
	out.Results["person"] = people
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) similar(w http.ResponseWriter, r *http.Request) {
	it, ok := s.loadItem(w, r)
	if !ok {
		return
	}
	limit := 12
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	ids, err := s.store.SimilarIDs(r.Context(), it.ID, it.Kind, limit)
	if err != nil {
		s.internalError(w, err)
		return
	}
	items, err := s.browseIDs(r, ids)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
