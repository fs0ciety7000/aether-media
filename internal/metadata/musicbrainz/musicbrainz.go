// Package musicbrainz is the MusicBrainz (+ Cover Art Archive) remote
// provider for artists and albums (release groups).
//
// MusicBrainz policy: at most 1 request per second per client, and a
// meaningful User-Agent. The httpx.Client given to New must enforce both.
package musicbrainz

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/metadata/httpx"
)

const (
	defaultBaseURL     = "https://musicbrainz.org/ws/2"
	defaultCoverArtURL = "https://coverartarchive.org"
	searchLimit        = 10
	maxGenres          = 10
	maxTagGenres       = 5
)

// Config configures the provider. Zero values get defaults.
type Config struct {
	BaseURL     string // default https://musicbrainz.org/ws/2
	CoverArtURL string // default https://coverartarchive.org
}

// Provider implements metadata.RemoteProvider.
type Provider struct {
	cfg Config
	c   *httpx.Client
}

var _ metadata.RemoteProvider = (*Provider)(nil)

// New builds a provider. c must be rate-limited at 1 req/s and carry a
// proper User-Agent (MusicBrainz policy), e.g.
//
//	httpx.New(httpx.Options{UserAgent: "Aether/1.0 ( https://github.com/fs0ciety7000/aether-media )", RatePerSec: 1, CacheDir: dir})
func New(cfg Config, c *httpx.Client) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.CoverArtURL == "" {
		cfg.CoverArtURL = defaultCoverArtURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	cfg.CoverArtURL = strings.TrimRight(cfg.CoverArtURL, "/")
	return &Provider{cfg: cfg, c: c}
}

// Name implements metadata.RemoteProvider.
func (p *Provider) Name() string { return "musicbrainz" }

// Supports implements metadata.RemoteProvider.
func (p *Provider) Supports(kind string) bool {
	return kind == metadata.KindArtist || kind == metadata.KindAlbum
}

// Search implements metadata.RemoteProvider.
func (p *Provider) Search(ctx context.Context, q metadata.Query) ([]metadata.Candidate, error) {
	switch q.Kind {
	case metadata.KindArtist:
		return p.searchArtist(ctx, q)
	case metadata.KindAlbum:
		return p.searchAlbum(ctx, q)
	}
	return nil, fmt.Errorf("musicbrainz: unsupported kind %q", q.Kind)
}

// Fetch implements metadata.RemoteProvider.
func (p *Provider) Fetch(ctx context.Context, q metadata.Query) (*metadata.Result, error) {
	switch q.Kind {
	case metadata.KindArtist:
		id := q.ProviderIDs[metadata.IDMusicBrainzArtist]
		if id == "" {
			return nil, fmt.Errorf("%w: musicbrainz: no artist id", metadata.ErrNotFound)
		}
		a, err := p.artist(ctx, id)
		if err != nil {
			return nil, err
		}
		return mapArtist(a), nil
	case metadata.KindAlbum:
		id, err := p.releaseGroupID(ctx, q.ProviderIDs)
		if err != nil {
			return nil, err
		}
		rg, err := p.releaseGroup(ctx, id)
		if err != nil {
			return nil, err
		}
		return p.mapReleaseGroup(rg), nil
	}
	return nil, fmt.Errorf("musicbrainz: unsupported kind %q", q.Kind)
}

// ---- response shapes ----

type lifeSpan struct {
	Begin string `json:"begin"`
	End   string `json:"end"`
	Ended bool   `json:"ended"`
}

type counted struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type artist struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	SortName       string    `json:"sort-name"`
	Type           string    `json:"type"`
	Country        string    `json:"country"`
	Disambiguation string    `json:"disambiguation"`
	Score          int       `json:"score"`
	LifeSpan       lifeSpan  `json:"life-span"`
	Genres         []counted `json:"genres"`
	Tags           []counted `json:"tags"`
}

type artistCredit struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
	Artist     struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		SortName string `json:"sort-name"`
	} `json:"artist"`
}

type release struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Date   string `json:"date"`
	Status string `json:"status"`
}

type releaseGroup struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Score            int            `json:"score"`
	PrimaryType      string         `json:"primary-type"`
	FirstReleaseDate string         `json:"first-release-date"`
	Disambiguation   string         `json:"disambiguation"`
	ArtistCredit     []artistCredit `json:"artist-credit"`
	Releases         []release      `json:"releases"`
	Genres           []counted      `json:"genres"`
	Tags             []counted      `json:"tags"`
}

// ---- search ----

// luceneEscape escapes a value placed inside a quoted Lucene phrase.
func luceneEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// get calls the web service. inc is appended verbatim: MusicBrainz expects
// literal '+' between inc values.
func (p *Provider) get(ctx context.Context, path string, params url.Values, inc string, out any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("fmt", "json")
	u := p.cfg.BaseURL + path + "?" + params.Encode()
	if inc != "" {
		u += "&inc=" + inc
	}
	if err := p.c.GetJSON(ctx, u, nil, out); err != nil {
		return fmt.Errorf("musicbrainz: %w", err)
	}
	return nil
}

// Artist score: 0.6 × name similarity (best of name / sort-name) + 0.4 ×
// MusicBrainz's own relevance score.
func (p *Provider) searchArtist(ctx context.Context, q metadata.Query) ([]metadata.Candidate, error) {
	if id := q.ProviderIDs[metadata.IDMusicBrainzArtist]; id != "" {
		a, err := p.artist(ctx, id)
		switch {
		case err == nil:
			return []metadata.Candidate{artistCandidate(a, 1)}, nil
		case !errors.Is(err, metadata.ErrNotFound):
			return nil, err
		}
	}
	if q.Title == "" {
		return nil, nil
	}
	var r struct {
		Artists []artist `json:"artists"`
	}
	err := p.get(ctx, "/artist", url.Values{
		"query": {`artist:"` + luceneEscape(q.Title) + `"`},
		"limit": {strconv.Itoa(searchLimit)},
	}, "", &r)
	if err != nil {
		return nil, err
	}
	cs := make([]metadata.Candidate, 0, len(r.Artists))
	for i := range r.Artists {
		a := &r.Artists[i]
		var w httpx.Weighted
		w.Add(0.6, max(httpx.TitleSimilarity(q.Title, a.Name), httpx.TitleSimilarity(q.Title, a.SortName)))
		w.Add(0.4, float64(a.Score)/100)
		cs = append(cs, artistCandidate(a, w.Score()))
	}
	return sortCandidates(cs), nil
}

func artistCandidate(a *artist, s float64) metadata.Candidate {
	return metadata.Candidate{
		Provider:    "musicbrainz",
		ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzArtist: a.ID},
		Title:       a.Name,
		Year:        yearOf(a.LifeSpan.Begin),
		Overview:    artistBlurb(a),
		Score:       s,
	}
}

// artistBlurb is a short disambiguation line for the identify UI
// ("Group, FR — French electronic duo"), never stored as an overview.
func artistBlurb(a *artist) string {
	var parts []string
	if a.Type != "" {
		parts = append(parts, a.Type)
	}
	if a.Country != "" {
		parts = append(parts, a.Country)
	}
	s := strings.Join(parts, ", ")
	if a.Disambiguation != "" {
		if s != "" {
			s += " — "
		}
		s += a.Disambiguation
	}
	return s
}

// Album score, weighted mean over known components: 0.55 × title
// similarity, 0.15 × MusicBrainz score, 0.15 × album-artist similarity
// (when q.ParentTitle is set), 0.15 × year match (when both years known).
func (p *Provider) searchAlbum(ctx context.Context, q metadata.Query) ([]metadata.Candidate, error) {
	if id := q.ProviderIDs[metadata.IDMusicBrainzRelGrp]; id != "" {
		rg, err := p.releaseGroup(ctx, id)
		switch {
		case err == nil:
			return []metadata.Candidate{p.albumCandidate(rg, 1)}, nil
		case !errors.Is(err, metadata.ErrNotFound):
			return nil, err
		}
	}
	if q.Title == "" {
		return nil, nil
	}
	query := `releasegroup:"` + luceneEscape(q.Title) + `"`
	if q.ParentTitle != "" {
		query += ` AND artist:"` + luceneEscape(q.ParentTitle) + `"`
	}
	var r struct {
		ReleaseGroups []releaseGroup `json:"release-groups"`
	}
	if err := p.get(ctx, "/release-group", url.Values{"query": {query}, "limit": {strconv.Itoa(searchLimit)}}, "", &r); err != nil {
		return nil, err
	}
	cs := make([]metadata.Candidate, 0, len(r.ReleaseGroups))
	for i := range r.ReleaseGroups {
		rg := &r.ReleaseGroups[i]
		var w httpx.Weighted
		w.Add(0.55, httpx.TitleSimilarity(q.Title, rg.Title))
		w.Add(0.15, float64(rg.Score)/100)
		if q.ParentTitle != "" {
			w.Add(0.15, httpx.TitleSimilarity(q.ParentTitle, creditName(rg.ArtistCredit)))
		}
		if ys, ok := httpx.YearScore(q.Year, yearOf(rg.FirstReleaseDate)); ok {
			w.Add(0.15, ys)
		}
		cs = append(cs, p.albumCandidate(rg, w.Score()))
	}
	return sortCandidates(cs), nil
}

func (p *Provider) albumCandidate(rg *releaseGroup, s float64) metadata.Candidate {
	ov := creditName(rg.ArtistCredit)
	if rg.PrimaryType != "" {
		ov = strings.TrimPrefix(ov+" — "+rg.PrimaryType, " — ")
	}
	return metadata.Candidate{
		Provider:    "musicbrainz",
		ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzRelGrp: rg.ID},
		Title:       rg.Title,
		Year:        yearOf(rg.FirstReleaseDate),
		Overview:    ov,
		ImageURL:    p.cover(rg.ID, "250"),
		Score:       s,
	}
}

func sortCandidates(cs []metadata.Candidate) []metadata.Candidate {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Score > cs[j].Score })
	if len(cs) > searchLimit {
		cs = cs[:searchLimit]
	}
	return cs
}

// ---- lookups ----

func (p *Provider) artist(ctx context.Context, mbid string) (*artist, error) {
	var a artist
	if err := p.get(ctx, "/artist/"+url.PathEscape(mbid), nil, "genres+tags+url-rels", &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (p *Provider) releaseGroup(ctx context.Context, mbid string) (*releaseGroup, error) {
	var rg releaseGroup
	if err := p.get(ctx, "/release-group/"+url.PathEscape(mbid), nil, "genres+artist-credits+releases", &rg); err != nil {
		return nil, err
	}
	return &rg, nil
}

// releaseGroupID returns the release group id, resolving it from a release
// id when only that is known (e.g. from file tags).
func (p *Provider) releaseGroupID(ctx context.Context, ids metadata.ProviderIDs) (string, error) {
	if id := ids[metadata.IDMusicBrainzRelGrp]; id != "" {
		return id, nil
	}
	rel := ids[metadata.IDMusicBrainzRelease]
	if rel == "" {
		return "", fmt.Errorf("%w: musicbrainz: no release group id", metadata.ErrNotFound)
	}
	var r struct {
		ReleaseGroup struct {
			ID string `json:"id"`
		} `json:"release-group"`
	}
	if err := p.get(ctx, "/release/"+url.PathEscape(rel), nil, "release-groups", &r); err != nil {
		return "", err
	}
	if r.ReleaseGroup.ID == "" {
		return "", fmt.Errorf("%w: musicbrainz: release %s has no release group", metadata.ErrNotFound, rel)
	}
	return r.ReleaseGroup.ID, nil
}

// ---- mapping ----

func mapArtist(a *artist) *metadata.Result {
	r := &metadata.Result{
		Title:       a.Name,
		SortTitle:   a.SortName,
		Year:        yearOf(a.LifeSpan.Begin),
		Genres:      topGenres(a.Genres, a.Tags),
		ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzArtist: a.ID},
	}
	if fullDate(a.LifeSpan.Begin) {
		r.PremiereDate = a.LifeSpan.Begin
	}
	if a.LifeSpan.Ended && fullDate(a.LifeSpan.End) {
		r.EndDate = a.LifeSpan.End
	}
	return r
}

func (p *Provider) mapReleaseGroup(rg *releaseGroup) *metadata.Result {
	r := &metadata.Result{
		Title:       rg.Title,
		Year:        yearOf(rg.FirstReleaseDate),
		Genres:      topGenres(rg.Genres, rg.Tags),
		ProviderIDs: metadata.ProviderIDs{metadata.IDMusicBrainzRelGrp: rg.ID},
		Images:      []metadata.RemoteImage{{Type: metadata.ImagePoster, URL: p.cover(rg.ID, "500")}},
	}
	if fullDate(rg.FirstReleaseDate) {
		r.PremiereDate = rg.FirstReleaseDate
	}
	if rel := firstRelease(rg.Releases); rel != "" {
		r.ProviderIDs[metadata.IDMusicBrainzRelease] = rel
	}
	for i, ac := range rg.ArtistCredit {
		name := firstNonEmpty(ac.Name, ac.Artist.Name)
		if name == "" {
			continue
		}
		pr := metadata.Person{Name: name, Kind: "artist", Order: i}
		if ac.Artist.ID != "" {
			pr.ProviderIDs = metadata.ProviderIDs{metadata.IDMusicBrainzArtist: ac.Artist.ID}
		}
		r.People = append(r.People, pr)
	}
	return r
}

// firstRelease picks the earliest dated official release (any release when
// none is official), keeping MusicBrainz order among ties.
func firstRelease(rs []release) string {
	best := -1
	better := func(a, b release) bool {
		ao, bo := a.Status == "Official", b.Status == "Official"
		if ao != bo {
			return ao
		}
		if (a.Date == "") != (b.Date == "") {
			return a.Date != ""
		}
		return a.Date < b.Date
	}
	for i := range rs {
		if best < 0 || better(rs[i], rs[best]) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return rs[best].ID
}

// topGenres returns genres by vote count; when MusicBrainz has no genres,
// the most voted tags stand in.
func topGenres(genres, tags []counted) []string {
	src, limit := genres, maxGenres
	if len(src) == 0 {
		src, limit = tags, maxTagGenres
	}
	src = append([]counted(nil), src...)
	sort.SliceStable(src, func(i, j int) bool { return src[i].Count > src[j].Count })
	var out []string
	for _, g := range src {
		if g.Name == "" || (g.Count <= 0 && len(genres) == 0) {
			continue
		}
		out = append(out, titleCase(g.Name))
		if len(out) == limit {
			break
		}
	}
	return out
}

// titleCase turns MusicBrainz's lower-case genre names into display form
// ("hip hop" → "Hip Hop").
func titleCase(s string) string {
	b := []byte(s)
	up := true
	for i, c := range b {
		if up && c >= 'a' && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
		up = c == ' ' || c == '-' || c == '/' || c == '&'
	}
	return string(b)
}

func (p *Provider) cover(rgid, size string) string {
	return p.cfg.CoverArtURL + "/release-group/" + url.PathEscape(rgid) + "/front-" + size
}

func creditName(acs []artistCredit) string {
	var b strings.Builder
	for _, ac := range acs {
		b.WriteString(firstNonEmpty(ac.Name, ac.Artist.Name))
		b.WriteString(ac.JoinPhrase)
	}
	return strings.TrimSpace(b.String())
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}

func fullDate(d string) bool { return len(d) == 10 }

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
