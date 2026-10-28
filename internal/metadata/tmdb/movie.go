package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

// resolveID returns the TMDB id in ids, resolving imdb/tvdb through /find
// when needed. ErrNotFound when nothing usable is known.
func (p *Provider) resolveID(ctx context.Context, q metadata.Query, ids metadata.ProviderIDs, tv bool) (string, error) {
	if id := ids[metadata.IDTmdb]; id != "" {
		return id, nil
	}
	f, err := p.find(ctx, q, ids)
	if err != nil {
		return "", err
	}
	if f != nil {
		if tv && len(f.TVResults) > 0 {
			return itoa(f.TVResults[0].ID), nil
		}
		if !tv && len(f.MovieResults) > 0 {
			return itoa(f.MovieResults[0].ID), nil
		}
	}
	return "", fmt.Errorf("%w: tmdb: no usable id", metadata.ErrNotFound)
}

func (p *Provider) movieDetails(ctx context.Context, q metadata.Query, id string) (*movieDetails, error) {
	var m movieDetails
	err := p.get(ctx, "/movie/"+url.PathEscape(id), url.Values{
		"language":               {p.lang(q)},
		"append_to_response":     {"credits,images,release_dates,external_ids"},
		"include_image_language": {p.includeImageLanguage(q)},
	}, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (p *Provider) fetchMovie(ctx context.Context, q metadata.Query) (*metadata.Result, error) {
	id, err := p.resolveID(ctx, q, q.ProviderIDs, false)
	if err != nil {
		return nil, err
	}
	m, err := p.movieDetails(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if (m.Overview == "" || m.Tagline == "") && iso639(p.lang(q)) != "en" {
		var en movieDetails
		if err := p.get(ctx, "/movie/"+url.PathEscape(id), url.Values{"language": {"en-US"}}, &en); err == nil {
			if m.Overview == "" {
				m.Overview = en.Overview
			}
			if m.Tagline == "" {
				m.Tagline = en.Tagline
			}
		}
	}
	return p.mapMovie(q, m), nil
}

func (p *Provider) mapMovie(q metadata.Query, m *movieDetails) *metadata.Result {
	r := &metadata.Result{
		Title:           m.Title,
		OriginalTitle:   m.OriginalTitle,
		Overview:        m.Overview,
		Tagline:         m.Tagline,
		PremiereDate:    m.ReleaseDate,
		Year:            yearOf(m.ReleaseDate),
		RuntimeMs:       int64(m.Runtime) * 60_000,
		CommunityRating: m.VoteAverage,
		Genres:          names(m.Genres),
		Studios:         names(m.ProductionCompanies),
		ProviderIDs:     metadata.ProviderIDs{metadata.IDTmdb: itoa(m.ID)},
	}
	if imdb := firstNonEmpty(m.ExternalIDs.ImdbID, m.ImdbID); imdb != "" {
		r.ProviderIDs[metadata.IDImdb] = imdb
	}
	if t := m.ExternalIDs.TvdbID; t != nil && *t > 0 {
		r.ProviderIDs[metadata.IDTvdb] = strconv.FormatInt(*t, 10)
	}
	if c := m.BelongsToCollection; c != nil && c.ID > 0 {
		r.Collection = c.Name
		r.ProviderIDs[metadata.IDTmdbCollection] = itoa(c.ID)
	}
	r.OfficialRating = p.movieCertification(q, m)
	r.People = append(p.cast(m.Credits.Cast, metadata.Person{Kind: "actor"}), p.crew(m.Credits.Crew)...)
	r.Images = p.images(q, m.Images, m.PosterPath, m.BackdropPath)
	return r
}

// movieCertification prefers the query language's region, then US; within a
// country the theatrical (type 3) release first.
func (p *Provider) movieCertification(q metadata.Query, m *movieDetails) string {
	for _, country := range countries(q, p) {
		for _, res := range m.ReleaseDates.Results {
			if res.Country != country {
				continue
			}
			best := ""
			for _, rd := range res.ReleaseDates {
				if rd.Certification == "" {
					continue
				}
				if rd.Type == 3 {
					return rd.Certification
				}
				if best == "" {
					best = rd.Certification
				}
			}
			if best != "" {
				return best
			}
		}
	}
	return ""
}

func countries(q metadata.Query, p *Provider) []string {
	r := region(p.lang(q))
	if r == "US" || r == "" {
		return []string{"US"}
	}
	return []string{r, "US"}
}

// cast maps actors (or guest stars, per tmpl.Kind) sorted by billing order.
func (p *Provider) cast(cs []castMember, tmpl metadata.Person) []metadata.Person {
	cs = append([]castMember(nil), cs...)
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Order < cs[j].Order })
	if len(cs) > maxCast {
		cs = cs[:maxCast]
	}
	out := make([]metadata.Person, 0, len(cs))
	for i, c := range cs {
		role := c.Character
		if role == "" && len(c.Roles) > 0 {
			best := c.Roles[0]
			for _, r := range c.Roles[1:] {
				if r.EpisodeCount > best.EpisodeCount {
					best = r
				}
			}
			role = best.Character
		}
		pr := tmpl
		pr.Name, pr.Role, pr.Order = c.Name, role, i
		pr.ImageURL = p.image(imageSizePerson, c.ProfilePath)
		pr.ProviderIDs = metadata.ProviderIDs{metadata.IDTmdb: itoa(c.ID)}
		out = append(out, pr)
	}
	return out
}

// crewJobs maps a TMDB job to a person kind, with a per-kind limit.
var crewJobs = map[string]string{
	"Director":                "director",
	"Writer":                  "writer",
	"Screenplay":              "writer",
	"Original Music Composer": "composer",
	"Producer":                "producer",
}

var crewLimits = map[string]int{"director": 5, "writer": 10, "composer": 3, "producer": 5}

func (p *Provider) crew(cs []crewMember) []metadata.Person {
	var out []metadata.Person
	count := map[string]int{}
	seen := map[string]bool{}
	for _, c := range cs {
		kind, ok := crewJobs[c.Job]
		if !ok || count[kind] >= crewLimits[kind] {
			continue
		}
		key := kind + "/" + itoa(c.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, metadata.Person{
			Name:        c.Name,
			Role:        c.Job,
			Kind:        kind,
			Order:       count[kind],
			ImageURL:    p.image(imageSizePerson, c.ProfilePath),
			ProviderIDs: metadata.ProviderIDs{metadata.IDTmdb: itoa(c.ID)},
		})
		count[kind]++
	}
	return out
}

// images maps posters, backdrops and logos. Posters and logos prefer the
// configured languages in order; backdrops prefer textless ones.
func (p *Provider) images(q metadata.Query, imgs images, posterPath, backdropPath string) []metadata.RemoteImage {
	langs := p.imageLangs(q)
	textless := append([]string{""}, langs...)
	var out []metadata.RemoteImage
	out = append(out, p.imageList(metadata.ImagePoster, imgs.Posters, langs, posterPath)...)
	out = append(out, p.imageList(metadata.ImageBackdrop, imgs.Backdrops, textless, backdropPath)...)
	out = append(out, p.imageList(metadata.ImageLogo, imgs.Logos, langs, "")...)
	return out
}

func (p *Provider) imageList(typ string, imgs []image, langs []string, fallback string) []metadata.RemoteImage {
	if len(imgs) == 0 {
		if fallback == "" {
			return nil
		}
		return []metadata.RemoteImage{{Type: typ, URL: p.image(imageSizeOriginal, fallback)}}
	}
	rank := func(l string) int {
		for i, x := range langs {
			if x == l {
				return i
			}
		}
		return len(langs)
	}
	out := make([]metadata.RemoteImage, 0, len(imgs))
	for _, im := range imgs {
		if im.FilePath == "" {
			continue
		}
		lang := ""
		if im.Language != nil {
			lang = *im.Language
		}
		out = append(out, metadata.RemoteImage{
			Type:     typ,
			URL:      p.image(imageSizeOriginal, im.FilePath),
			Language: lang,
			Width:    im.Width,
			Height:   im.Height,
			Rating:   im.VoteAverage,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i].Language), rank(out[j].Language)
		if ri != rj {
			return ri < rj
		}
		if out[i].Rating != out[j].Rating {
			return out[i].Rating > out[j].Rating
		}
		return out[i].Width > out[j].Width
	})
	return out
}

func names(ns []named) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range ns {
		if n.Name != "" && !seen[n.Name] {
			seen[n.Name] = true
			out = append(out, n.Name)
		}
	}
	return out
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
