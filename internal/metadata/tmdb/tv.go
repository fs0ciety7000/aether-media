package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

func (p *Provider) tvDetails(ctx context.Context, q metadata.Query, id string) (*tvDetails, error) {
	var s tvDetails
	err := p.get(ctx, "/tv/"+url.PathEscape(id), url.Values{
		"language":               {p.lang(q)},
		"append_to_response":     {"aggregate_credits,images,content_ratings,external_ids"},
		"include_image_language": {p.includeImageLanguage(q)},
	}, &s)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (p *Provider) fetchSeries(ctx context.Context, q metadata.Query) (*metadata.Result, error) {
	id, err := p.resolveID(ctx, q, q.ProviderIDs, true)
	if err != nil {
		return nil, err
	}
	s, err := p.tvDetails(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if (s.Overview == "" || s.Tagline == "") && iso639(p.lang(q)) != "en" {
		var en tvDetails
		if err := p.get(ctx, "/tv/"+url.PathEscape(id), url.Values{"language": {"en-US"}}, &en); err == nil {
			if s.Overview == "" {
				s.Overview = en.Overview
			}
			if s.Tagline == "" {
				s.Tagline = en.Tagline
			}
		}
	}
	return p.mapSeries(q, s), nil
}

func seriesStatus(s string) string {
	switch s {
	case "Returning Series", "In Production":
		return "continuing"
	case "Ended", "Canceled", "Cancelled":
		return "ended"
	}
	return ""
}

func (p *Provider) mapSeries(q metadata.Query, s *tvDetails) *metadata.Result {
	r := &metadata.Result{
		Title:           s.Name,
		OriginalTitle:   s.OriginalName,
		Overview:        s.Overview,
		Tagline:         s.Tagline,
		PremiereDate:    s.FirstAirDate,
		Year:            yearOf(s.FirstAirDate),
		Status:          seriesStatus(s.Status),
		CommunityRating: s.VoteAverage,
		Genres:          names(s.Genres),
		Studios:         names(append(append([]named(nil), s.Networks...), s.ProductionCompanies...)),
		ProviderIDs:     metadata.ProviderIDs{metadata.IDTmdb: itoa(s.ID)},
	}
	if r.Status == "ended" {
		r.EndDate = s.LastAirDate
	}
	for _, rt := range s.EpisodeRunTime {
		if rt > 0 {
			r.RuntimeMs = int64(rt) * 60_000
			break
		}
	}
	if s.ExternalIDs.ImdbID != "" {
		r.ProviderIDs[metadata.IDImdb] = s.ExternalIDs.ImdbID
	}
	if t := s.ExternalIDs.TvdbID; t != nil && *t > 0 {
		r.ProviderIDs[metadata.IDTvdb] = strconv.FormatInt(*t, 10)
	}
	for _, country := range countries(q, p) {
		for _, cr := range s.ContentRatings.Results {
			if cr.Country == country && cr.Rating != "" && r.OfficialRating == "" {
				r.OfficialRating = cr.Rating
			}
		}
	}
	r.People = p.cast(s.AggregateCredits.Cast, metadata.Person{Kind: "actor"})
	for i, c := range s.CreatedBy {
		r.People = append(r.People, metadata.Person{
			Name:        c.Name,
			Role:        "Creator",
			Kind:        "writer",
			Order:       i,
			ImageURL:    p.image(imageSizePerson, c.ProfilePath),
			ProviderIDs: metadata.ProviderIDs{metadata.IDTmdb: itoa(c.ID)},
		})
	}
	r.Images = p.images(q, s.Images, s.PosterPath, s.BackdropPath)
	return r
}

// season returns the raw season, from the in-memory cache when possible so
// that fetching every episode of a season costs one request.
func (p *Provider) season(ctx context.Context, q metadata.Query, seriesID string, n int) (*tvSeason, error) {
	if n < 0 {
		return nil, fmt.Errorf("%w: tmdb: invalid season number %d", metadata.ErrNotFound, n)
	}
	key := seriesID + "/" + itoa(n) + "/" + p.lang(q)
	if s := p.cachedSeason(key); s != nil {
		return s, nil
	}
	path := "/tv/" + url.PathEscape(seriesID) + "/season/" + itoa(n)
	var s tvSeason
	err := p.get(ctx, path, url.Values{
		"language":               {p.lang(q)},
		"append_to_response":     {"images"},
		"include_image_language": {p.includeImageLanguage(q)},
	}, &s)
	if err != nil {
		return nil, err
	}
	if iso639(p.lang(q)) != "en" && missingOverview(&s) {
		var en tvSeason
		if err := p.get(ctx, path, url.Values{"language": {"en-US"}}, &en); err == nil {
			fillOverviews(&s, &en)
		}
	}
	p.storeSeason(key, &s)
	return &s, nil
}

func missingOverview(s *tvSeason) bool {
	if s.Overview == "" {
		return true
	}
	for _, e := range s.Episodes {
		if e.Overview == "" {
			return true
		}
	}
	return false
}

func fillOverviews(dst, en *tvSeason) {
	if dst.Overview == "" {
		dst.Overview = en.Overview
	}
	byNum := map[int]string{}
	for _, e := range en.Episodes {
		byNum[e.EpisodeNumber] = e.Overview
	}
	for i := range dst.Episodes {
		if dst.Episodes[i].Overview == "" {
			dst.Episodes[i].Overview = byNum[dst.Episodes[i].EpisodeNumber]
		}
	}
}

func (p *Provider) fetchSeason(ctx context.Context, q metadata.Query) (*metadata.Result, error) {
	seriesID, err := p.resolveID(ctx, q, q.ParentIDs, true)
	if err != nil {
		return nil, err
	}
	s, err := p.season(ctx, q, seriesID, q.IndexNumber)
	if err != nil {
		return nil, err
	}
	return p.mapSeason(q, s), nil
}

func (p *Provider) mapSeason(q metadata.Query, s *tvSeason) *metadata.Result {
	r := &metadata.Result{
		Title:           s.Name,
		Overview:        s.Overview,
		PremiereDate:    s.AirDate,
		Year:            yearOf(s.AirDate),
		CommunityRating: s.VoteAverage,
		IndexNumber:     s.SeasonNumber,
		ProviderIDs:     metadata.ProviderIDs{metadata.IDTmdb: itoa(s.ID)},
		Images:          p.imageList(metadata.ImagePoster, s.Images.Posters, p.imageLangs(q), s.PosterPath),
	}
	if len(s.Episodes) > 0 {
		r.Children = make(map[int]*metadata.Result, len(s.Episodes))
		for i := range s.Episodes {
			e := &s.Episodes[i]
			r.Children[e.EpisodeNumber] = p.mapEpisode(e)
		}
	}
	return r
}

func (p *Provider) mapEpisode(e *tvEpisode) *metadata.Result {
	r := &metadata.Result{
		Title:           e.Name,
		Overview:        e.Overview,
		PremiereDate:    e.AirDate,
		Year:            yearOf(e.AirDate),
		RuntimeMs:       int64(e.Runtime) * 60_000,
		CommunityRating: e.VoteAverage,
		IndexNumber:     e.EpisodeNumber,
		ParentIndex:     e.SeasonNumber,
		ProviderIDs:     metadata.ProviderIDs{metadata.IDTmdb: itoa(e.ID)},
	}
	if e.StillPath != "" {
		r.Images = []metadata.RemoteImage{{Type: metadata.ImageThumb, URL: p.image(imageSizeOriginal, e.StillPath)}}
	}
	r.People = append(p.crew(e.Crew), p.cast(e.GuestStars, metadata.Person{Kind: "guest"})...)
	return r
}

func (p *Provider) fetchEpisode(ctx context.Context, q metadata.Query) (*metadata.Result, error) {
	if q.IndexNumber <= 0 {
		return nil, fmt.Errorf("%w: tmdb: episode number unknown", metadata.ErrNotFound)
	}
	seriesID, err := p.resolveID(ctx, q, q.ParentIDs, true)
	if err != nil {
		return nil, err
	}
	s, err := p.season(ctx, q, seriesID, q.ParentIndex)
	if err != nil {
		return nil, err
	}
	for i := range s.Episodes {
		if s.Episodes[i].EpisodeNumber == q.IndexNumber {
			return p.mapEpisode(&s.Episodes[i]), nil
		}
	}
	return nil, fmt.Errorf("%w: tmdb: series %s has no episode S%02dE%02d",
		metadata.ErrNotFound, seriesID, q.ParentIndex, q.IndexNumber)
}
