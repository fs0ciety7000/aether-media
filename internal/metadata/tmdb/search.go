package tmdb

import (
	"context"
	"errors"
	"net/url"
	"sort"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/metadata/httpx"
)

// Score weights. Components that are unknown (no year on either side, no
// popularity) drop out and the rest are renormalized, so with no year an
// exact title still scores ~1.
const (
	weightTitle      = 0.70
	weightYear       = 0.25
	weightPopularity = 0.05
)

// score rates a hit against the query: best title similarity over the
// localized and original titles, year proximity, and popularity relative to
// the best-known hit of the same result page (tie-break).
func score(q metadata.Query, title, original string, year int, pop, maxPop float64) float64 {
	var w httpx.Weighted
	w.Add(weightTitle, max(httpx.TitleSimilarity(q.Title, title), httpx.TitleSimilarity(q.Title, original)))
	if ys, ok := httpx.YearScore(q.Year, year); ok {
		w.Add(weightYear, ys)
	}
	if maxPop > 0 {
		w.Add(weightPopularity, pop/maxPop)
	}
	return w.Score()
}

func sortCandidates(cs []metadata.Candidate) []metadata.Candidate {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Score > cs[j].Score })
	if len(cs) > maxCandidates {
		cs = cs[:maxCandidates]
	}
	return cs
}

func (p *Provider) movieCandidate(h movieHit, s float64) metadata.Candidate {
	return metadata.Candidate{
		Provider:    p.Name(),
		ProviderIDs: metadata.ProviderIDs{metadata.IDTmdb: itoa(h.ID)},
		Title:       h.Title,
		Year:        yearOf(h.ReleaseDate),
		Overview:    h.Overview,
		ImageURL:    p.image(imageSizeCandidate, h.PosterPath),
		Score:       s,
	}
}

func (p *Provider) tvCandidate(h tvHit, s float64) metadata.Candidate {
	return metadata.Candidate{
		Provider:    p.Name(),
		ProviderIDs: metadata.ProviderIDs{metadata.IDTmdb: itoa(h.ID)},
		Title:       h.Name,
		Year:        yearOf(h.FirstAirDate),
		Overview:    h.Overview,
		ImageURL:    p.image(imageSizeCandidate, h.PosterPath),
		Score:       s,
	}
}

// find resolves imdb/tvdb ids through /find. It returns nothing (and no
// error) when q has no such id or TMDB does not know it.
func (p *Provider) find(ctx context.Context, q metadata.Query, ids metadata.ProviderIDs) (*findResp, error) {
	for _, src := range []struct{ key, source string }{
		{metadata.IDImdb, "imdb_id"},
		{metadata.IDTvdb, "tvdb_id"},
	} {
		id := ids[src.key]
		if id == "" {
			continue
		}
		var r findResp
		err := p.get(ctx, "/find/"+url.PathEscape(id), url.Values{
			"external_source": {src.source},
			"language":        {p.lang(q)},
		}, &r)
		if errors.Is(err, metadata.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(r.MovieResults) > 0 || len(r.TVResults) > 0 {
			return &r, nil
		}
	}
	return nil, nil
}

func (p *Provider) searchMovie(ctx context.Context, q metadata.Query) ([]metadata.Candidate, error) {
	if id := q.ProviderIDs[metadata.IDTmdb]; id != "" {
		m, err := p.movieDetails(ctx, q, id)
		switch {
		case err == nil:
			h := movieHit{ID: m.ID, Title: m.Title, ReleaseDate: m.ReleaseDate, Overview: m.Overview, PosterPath: m.PosterPath}
			return []metadata.Candidate{p.movieCandidate(h, 1)}, nil
		case !errors.Is(err, metadata.ErrNotFound):
			return nil, err
		}
	}
	f, err := p.find(ctx, q, q.ProviderIDs)
	if err != nil {
		return nil, err
	}
	if f != nil && len(f.MovieResults) > 0 {
		return p.scoreMovies(q, f.MovieResults, true), nil
	}
	if q.Title == "" {
		return nil, nil
	}

	base := url.Values{"query": {q.Title}, "language": {p.lang(q)}, "include_adult": {"false"}}
	var attempts []url.Values
	if q.Year > 0 {
		for _, k := range []string{"year", "primary_release_year"} {
			v := cloneValues(base)
			v.Set(k, itoa(q.Year))
			attempts = append(attempts, v)
		}
	}
	attempts = append(attempts, base)
	for _, params := range attempts {
		var r movieSearch
		if err := p.get(ctx, "/search/movie", params, &r); err != nil {
			return nil, err
		}
		if len(r.Results) > 0 {
			return p.scoreMovies(q, r.Results, false), nil
		}
	}
	return nil, nil
}

// scoreMovies scores hits; byID hits came from an exact external id match
// and score 1.
func (p *Provider) scoreMovies(q metadata.Query, hits []movieHit, byID bool) []metadata.Candidate {
	maxPop := 0.0
	for _, h := range hits {
		maxPop = max(maxPop, h.Popularity)
	}
	cs := make([]metadata.Candidate, 0, len(hits))
	for _, h := range hits {
		s := 1.0
		if !byID {
			s = score(q, h.Title, h.OriginalTitle, yearOf(h.ReleaseDate), h.Popularity, maxPop)
		}
		cs = append(cs, p.movieCandidate(h, s))
	}
	return sortCandidates(cs)
}

func (p *Provider) searchSeries(ctx context.Context, q metadata.Query) ([]metadata.Candidate, error) {
	if id := q.ProviderIDs[metadata.IDTmdb]; id != "" {
		s, err := p.tvDetails(ctx, q, id)
		switch {
		case err == nil:
			h := tvHit{ID: s.ID, Name: s.Name, FirstAirDate: s.FirstAirDate, Overview: s.Overview, PosterPath: s.PosterPath}
			return []metadata.Candidate{p.tvCandidate(h, 1)}, nil
		case !errors.Is(err, metadata.ErrNotFound):
			return nil, err
		}
	}
	f, err := p.find(ctx, q, q.ProviderIDs)
	if err != nil {
		return nil, err
	}
	if f != nil && len(f.TVResults) > 0 {
		return p.scoreSeries(q, f.TVResults, true), nil
	}
	if q.Title == "" {
		return nil, nil
	}

	base := url.Values{"query": {q.Title}, "language": {p.lang(q)}, "include_adult": {"false"}}
	var attempts []url.Values
	if q.Year > 0 {
		v := cloneValues(base)
		v.Set("first_air_date_year", itoa(q.Year))
		attempts = append(attempts, v)
	}
	attempts = append(attempts, base)
	for _, params := range attempts {
		var r tvSearch
		if err := p.get(ctx, "/search/tv", params, &r); err != nil {
			return nil, err
		}
		if len(r.Results) > 0 {
			return p.scoreSeries(q, r.Results, false), nil
		}
	}
	return nil, nil
}

func (p *Provider) scoreSeries(q metadata.Query, hits []tvHit, byID bool) []metadata.Candidate {
	maxPop := 0.0
	for _, h := range hits {
		maxPop = max(maxPop, h.Popularity)
	}
	cs := make([]metadata.Candidate, 0, len(hits))
	for _, h := range hits {
		s := 1.0
		if !byID {
			s = score(q, h.Name, h.OriginalName, yearOf(h.FirstAirDate), h.Popularity, maxPop)
		}
		cs = append(cs, p.tvCandidate(h, s))
	}
	return sortCandidates(cs)
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
