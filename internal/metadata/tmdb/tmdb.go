// Package tmdb is the TMDB (themoviedb.org, API v3) remote provider for
// movies, series, seasons and episodes.
package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/metadata/httpx"
)

const (
	defaultBaseURL      = "https://api.themoviedb.org/3"
	defaultImageBaseURL = "https://image.tmdb.org/t/p/"
	defaultLanguage     = "fr-FR"
	imageSizeOriginal   = "original"
	imageSizePerson     = "w185"
	imageSizeCandidate  = "w342"
	maxCandidates       = 10
	maxCast             = 30
	seasonCacheTTL      = 10 * time.Minute
	seasonCacheMax      = 64
)

// Config configures the provider. Zero values get defaults.
type Config struct {
	// APIKey is a v3 key (sent as api_key) or a v4 read access token (a JWT
	// starting with "eyJ", sent as "Authorization: Bearer"). Empty disables
	// the provider.
	APIKey string
	// Language for texts, e.g. "fr-FR" (default). Query.Language overrides it.
	Language string
	// Country for age ratings and release dates ("FR"); default: the
	// language's region.
	Country string
	// ImageLanguages ordered by preference ("null" = no text); default is
	// Language's ISO 639-1 code, "en", "null".
	ImageLanguages []string
	BaseURL        string // default https://api.themoviedb.org/3
	ImageBaseURL   string // default https://image.tmdb.org/t/p/
}

// Provider implements metadata.RemoteProvider.
type Provider struct {
	cfg  Config
	c    *httpx.Client
	live atomic.Pointer[liveSettings]

	mu      sync.Mutex
	seasons map[string]seasonEntry
	now     func() time.Time
}

type seasonEntry struct {
	at time.Time
	s  *tvSeason
}

var _ metadata.RemoteProvider = (*Provider)(nil)

// New builds a provider. c should carry a modest rate limit (TMDB allows
// ~40 req/s; 20 is a polite default) and a cache directory.
func New(cfg Config, c *httpx.Client) *Provider {
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.Language == "" {
		cfg.Language = defaultLanguage
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.ImageBaseURL == "" {
		cfg.ImageBaseURL = defaultImageBaseURL
	}
	if !strings.HasSuffix(cfg.ImageBaseURL, "/") {
		cfg.ImageBaseURL += "/"
	}
	p := &Provider{
		cfg:     cfg,
		c:       c,
		seasons: map[string]seasonEntry{},
		now:     time.Now,
	}
	p.live.Store(&liveSettings{apiKey: cfg.APIKey, bearer: isBearer(cfg.APIKey), language: cfg.Language, country: strings.ToUpper(cfg.Country)})
	return p
}

// liveSettings can change while the server runs (admin settings).
type liveSettings struct {
	apiKey   string
	bearer   bool
	language string
	country  string
}

func isBearer(key string) bool { return strings.HasPrefix(key, "eyJ") }

func (p *Provider) cur() *liveSettings { return p.live.Load() }

// SetAPIKey replaces the API key (empty disables the provider).
func (p *Provider) SetAPIKey(key string) {
	old := p.cur()
	key = strings.TrimSpace(key)
	p.live.Store(&liveSettings{apiKey: key, bearer: isBearer(key), language: old.language, country: old.country})
}

// SetLocale changes the default language and the country used for age
// ratings and release dates ("" = the language's region).
func (p *Provider) SetLocale(language, country string) {
	old := p.cur()
	if language == "" {
		language = defaultLanguage
	}
	p.live.Store(&liveSettings{apiKey: old.apiKey, bearer: old.bearer, language: language, country: strings.ToUpper(country)})
}

// ErrInvalidKey is returned by CheckKey when TMDB refuses the key.
var ErrInvalidKey = errors.New("tmdb: invalid API key")

// CheckKey asks TMDB whether key is valid, without caching nor affecting
// the provider. ErrInvalidKey when refused; any other error means TMDB
// could not be reached.
func (p *Provider) CheckKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrInvalidKey
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u := p.cfg.BaseURL + "/authentication"
	if !isBearer(key) {
		u += "?api_key=" + url.QueryEscape(key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if isBearer(key) {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: %s", httpx.Redact(err.Error()))
	}
	_ = res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrInvalidKey
	}
	return fmt.Errorf("tmdb: status %d", res.StatusCode)
}

// Name implements metadata.RemoteProvider.
func (p *Provider) Name() string { return "tmdb" }

// Enabled reports whether an API key is configured.
func (p *Provider) Enabled() bool { return p.cur().apiKey != "" }

// Supports implements metadata.RemoteProvider.
func (p *Provider) Supports(kind string) bool {
	switch kind {
	case metadata.KindMovie, metadata.KindSeries, metadata.KindSeason, metadata.KindEpisode:
		return true
	}
	return false
}

// Search implements metadata.RemoteProvider.
func (p *Provider) Search(ctx context.Context, q metadata.Query) ([]metadata.Candidate, error) {
	if !p.Enabled() {
		return nil, errDisabled
	}
	switch q.Kind {
	case metadata.KindMovie:
		return p.searchMovie(ctx, q)
	case metadata.KindSeries:
		return p.searchSeries(ctx, q)
	case metadata.KindSeason, metadata.KindEpisode:
		// Identified through their series: offer the single exact hit.
		r, err := p.Fetch(ctx, q)
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		c := metadata.Candidate{Provider: p.Name(), ProviderIDs: r.ProviderIDs, Title: r.Title,
			Year: r.Year, Overview: r.Overview, Score: 1}
		if len(r.Images) > 0 {
			c.ImageURL = r.Images[0].URL
		}
		return []metadata.Candidate{c}, nil
	}
	return nil, fmt.Errorf("tmdb: unsupported kind %q", q.Kind)
}

// Fetch implements metadata.RemoteProvider.
func (p *Provider) Fetch(ctx context.Context, q metadata.Query) (*metadata.Result, error) {
	if !p.Enabled() {
		return nil, errDisabled
	}
	switch q.Kind {
	case metadata.KindMovie:
		return p.fetchMovie(ctx, q)
	case metadata.KindSeries:
		return p.fetchSeries(ctx, q)
	case metadata.KindSeason:
		return p.fetchSeason(ctx, q)
	case metadata.KindEpisode:
		return p.fetchEpisode(ctx, q)
	}
	return nil, fmt.Errorf("tmdb: unsupported kind %q", q.Kind)
}

var errDisabled = fmt.Errorf("%w: tmdb: no API key configured", metadata.ErrUnavailable)

// ---- request plumbing ----

func (p *Provider) get(ctx context.Context, path string, params url.Values, out any) error {
	if params == nil {
		params = url.Values{}
	}
	var h http.Header
	if cur := p.cur(); cur.bearer {
		h = http.Header{"Authorization": {"Bearer " + cur.apiKey}}
	} else {
		params.Set("api_key", cur.apiKey)
	}
	u := p.cfg.BaseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	if err := p.c.GetJSON(ctx, u, h, out); err != nil {
		return fmt.Errorf("tmdb: %w", err)
	}
	return nil
}

func (p *Provider) lang(q metadata.Query) string {
	if q.Language != "" {
		return q.Language
	}
	return p.cur().language
}

// imageLangs returns the preference order of image languages for q, using
// "" for "no text" (TMDB's null).
func (p *Provider) imageLangs(q metadata.Query) []string {
	src := p.cfg.ImageLanguages
	if len(src) == 0 {
		src = []string{iso639(p.lang(q)), "en", "null"}
	}
	var out []string
	seen := map[string]bool{}
	for _, l := range src {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "null" {
			l = ""
		}
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

func (p *Provider) includeImageLanguage(q metadata.Query) string {
	ls := p.imageLangs(q)
	parts := make([]string, len(ls))
	for i, l := range ls {
		if l == "" {
			l = "null"
		}
		parts[i] = l
	}
	return strings.Join(parts, ",")
}

// iso639 extracts "fr" from "fr-FR".
func iso639(lang string) string {
	l, _, _ := strings.Cut(lang, "-")
	return strings.ToLower(l)
}

// region extracts "FR" from "fr-FR" (or "FR" from "fr").
func region(lang string) string {
	if _, r, ok := strings.Cut(lang, "-"); ok && r != "" {
		return strings.ToUpper(r)
	}
	return strings.ToUpper(lang)
}

func (p *Provider) image(size, path string) string {
	if path == "" {
		return ""
	}
	return p.cfg.ImageBaseURL + size + path
}

func itoa(i int) string { return strconv.Itoa(i) }

// yearOf parses the year of a "YYYY-MM-DD" date.
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

// ---- in-memory season cache (episodes of one season share one request) ----

func (p *Provider) cachedSeason(key string) *tvSeason {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.seasons[key]
	if !ok || p.now().Sub(e.at) > seasonCacheTTL {
		return nil
	}
	return e.s
}

func (p *Provider) storeSeason(key string, s *tvSeason) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if len(p.seasons) >= seasonCacheMax {
		for k, e := range p.seasons {
			if now.Sub(e.at) > seasonCacheTTL {
				delete(p.seasons, k)
			}
		}
		if len(p.seasons) >= seasonCacheMax {
			var oldest string
			for k, e := range p.seasons {
				if oldest == "" || e.at.Before(p.seasons[oldest].at) {
					oldest = k
				}
			}
			delete(p.seasons, oldest)
		}
	}
	p.seasons[key] = seasonEntry{at: now, s: s}
}
