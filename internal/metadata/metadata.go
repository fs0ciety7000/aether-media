// Package metadata defines what providers return and how the refresher
// combines them. Providers live in sub-packages (nfo, tmdb, musicbrainz).
package metadata

import (
	"context"
	"errors"
)

// Item kinds providers deal with (mirror items.kind).
const (
	KindMovie   = "movie"
	KindSeries  = "series"
	KindSeason  = "season"
	KindEpisode = "episode"
	KindArtist  = "artist"
	KindAlbum   = "album"
	KindTrack   = "track"
)

// Image types.
const (
	ImagePoster   = "poster"   // 2:3 (movie, series, season) or 1:1 cover (album, artist)
	ImageBackdrop = "backdrop" // 16:9 fanart
	ImageLogo     = "logo"     // transparent title logo
	ImageThumb    = "thumb"    // 16:9 still (episode) or landscape art
	ImageBanner   = "banner"
)

// Provider id keys used in ProviderIDs maps.
const (
	IDTmdb               = "tmdb"
	IDImdb               = "imdb"
	IDTvdb               = "tvdb"
	IDTmdbCollection     = "tmdbcollection"
	IDMusicBrainzArtist  = "musicbrainzartist"
	IDMusicBrainzRelGrp  = "musicbrainzreleasegroup"
	IDMusicBrainzRelease = "musicbrainzrelease"
	IDMusicBrainzTrack   = "musicbrainztrack"
)

// ProviderIDs maps an id key (IDTmdb...) to its value.
type ProviderIDs map[string]string

// Person credit.
type Person struct {
	Name        string      `json:"name"`
	Role        string      `json:"role,omitempty"` // character for actors, job otherwise
	Kind        string      `json:"kind"`           // actor | director | writer | producer | composer | guest
	Order       int         `json:"order"`
	ImageURL    string      `json:"imageUrl,omitempty"`
	ProviderIDs ProviderIDs `json:"providerIds,omitempty"`
}

// RemoteImage is an image offered by a provider. URL is http(s) for remote
// providers or an absolute local path for local ones (Local = true).
type RemoteImage struct {
	Type     string  `json:"type"`
	URL      string  `json:"url"`
	Local    bool    `json:"local,omitempty"`
	Language string  `json:"language,omitempty"` // ISO 639-1, "" = no text
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	Rating   float64 `json:"rating,omitempty"` // provider score, higher is better
}

// Result is what a provider knows about one item. Zero values mean
// "unknown" and never overwrite known data during merge.
type Result struct {
	Title           string        `json:"title,omitempty"`
	OriginalTitle   string        `json:"originalTitle,omitempty"`
	SortTitle       string        `json:"sortTitle,omitempty"`
	Overview        string        `json:"overview,omitempty"`
	Tagline         string        `json:"tagline,omitempty"`
	Year            int           `json:"year,omitempty"`
	PremiereDate    string        `json:"premiereDate,omitempty"` // YYYY-MM-DD
	EndDate         string        `json:"endDate,omitempty"`      // series end
	Status          string        `json:"status,omitempty"`       // series: continuing | ended
	RuntimeMs       int64         `json:"runtimeMs,omitempty"`
	OfficialRating  string        `json:"officialRating,omitempty"` // e.g. "12", "PG-13"
	CommunityRating float64       `json:"communityRating,omitempty"`
	Genres          []string      `json:"genres,omitempty"`
	Studios         []string      `json:"studios,omitempty"`
	People          []Person      `json:"people,omitempty"`
	ProviderIDs     ProviderIDs   `json:"providerIds,omitempty"`
	Images          []RemoteImage `json:"images,omitempty"`
	Collection      string        `json:"collection,omitempty"` // e.g. "Kill Bill - Saga"
	IndexNumber     int           `json:"indexNumber,omitempty"`
	ParentIndex     int           `json:"parentIndex,omitempty"`
	// Children carries per-episode results when a provider returns a whole
	// season at once (keyed by episode number), or per-track for albums.
	Children map[int]*Result `json:"children,omitempty"`
}

// Query describes an item to identify or fetch.
type Query struct {
	Kind        string
	Title       string
	Year        int
	Path        string // file (movie/episode/track) or folder (series/season/album)
	Language    string // BCP-47, e.g. "fr-FR"
	ProviderIDs ProviderIDs
	// Context for children: series ids for seasons/episodes, album for tracks.
	ParentIDs   ProviderIDs
	ParentTitle string // series title / album artist
	IndexNumber int    // season number, episode number, track number
	ParentIndex int    // season number for episodes
}

// Candidate is a search hit used to identify an item.
type Candidate struct {
	Provider    string      `json:"provider"`
	ProviderIDs ProviderIDs `json:"providerIds"`
	Title       string      `json:"title"`
	Year        int         `json:"year,omitempty"`
	Overview    string      `json:"overview,omitempty"`
	ImageURL    string      `json:"imageUrl,omitempty"`
	Score       float64     `json:"score"` // 0..1, computed by the provider against the query
}

// LocalProvider reads metadata stored next to the media (NFO, images).
type LocalProvider interface {
	Name() string
	Supports(kind string) bool
	// Local returns nil, nil when nothing is found.
	Local(ctx context.Context, q Query) (*Result, error)
}

// RemoteProvider queries an online database.
type RemoteProvider interface {
	Name() string
	Supports(kind string) bool
	// Search returns candidates ordered by Score descending.
	Search(ctx context.Context, q Query) ([]Candidate, error)
	// Fetch returns full metadata for q.ProviderIDs (and q.ParentIDs for
	// seasons/episodes). ErrNotFound when the ids are unknown.
	Fetch(ctx context.Context, q Query) (*Result, error)
}

var (
	// ErrNotFound: the provider has no such item.
	ErrNotFound = errors.New("metadata: not found")
	// ErrUnavailable: provider disabled (no API key) or unreachable; retry later.
	ErrUnavailable = errors.New("metadata: provider unavailable")
)
