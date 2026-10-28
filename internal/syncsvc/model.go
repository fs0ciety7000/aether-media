// Package syncsvc synchronises users' viewing data with Trakt and Simkl:
// per-user OAuth links (Trakt device code, Simkl PIN), live scrobbling from
// player events, and a three-way merge of watched history, resume points,
// watchlist and ratings run from the persistent job queue.
//
// Tokens are sealed with internal/secret and never leave the package except
// towards the provider they belong to; errors and logs never contain them.
package syncsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Feature is a synchronised function a user can switch on or off.
type Feature string

const (
	FeatScrobble  Feature = "scrobble"
	FeatHistory   Feature = "history"
	FeatProgress  Feature = "progress"
	FeatWatchlist Feature = "watchlist"
	FeatRatings   Feature = "ratings"
)

// AllFeatures in display order.
var AllFeatures = []Feature{FeatScrobble, FeatHistory, FeatProgress, FeatWatchlist, FeatRatings}

// mergeFeatures are the features handled by the periodic synchronisation.
var mergeFeatures = []Feature{FeatHistory, FeatProgress, FeatWatchlist, FeatRatings}

// defaultHandles is the item kinds each feature covers on Trakt.
func defaultHandles(f Feature, kind string) bool {
	switch f {
	case FeatHistory, FeatProgress, FeatScrobble:
		return kind == "movie" || kind == "episode"
	case FeatWatchlist:
		return kind == "movie" || kind == "show"
	case FeatRatings:
		return true
	}
	return false
}

// Action is a scrobble event.
type Action string

const (
	ActStart Action = "start"
	ActPause Action = "pause"
	ActStop  Action = "stop"
)

// IDs identify a movie or show on external services. Empty = unknown.
type IDs struct {
	Trakt string `json:"trakt,omitempty"`
	Simkl string `json:"simkl,omitempty"`
	Tmdb  string `json:"tmdb,omitempty"`
	Imdb  string `json:"imdb,omitempty"`
	Tvdb  string `json:"tvdb,omitempty"`
}

func (i IDs) empty() bool {
	return i.Tmdb == "" && i.Imdb == "" && i.Tvdb == "" && i.Trakt == "" && i.Simkl == ""
}

// Ref designates a movie, a show or an episode (show IDs + numbers).
type Ref struct {
	Kind    string `json:"kind"` // movie | show | episode
	IDs     IDs    `json:"ids"`
	Season  int    `json:"season,omitempty"`
	Episode int    `json:"episode,omitempty"`
}

// Entry is one remote record: a watched item, a resume point, a watchlist
// entry or a rating.
type Entry struct {
	Ref
	At        time.Time // watched_at, paused_at, listed_at, rated_at
	Value     float64   // rating (1-10) or progress (0-100)
	RemoteRef string    // provider-specific id (Trakt playback id)
}

// Token is an OAuth token pair. Expiry is zero when the token does not expire.
type Token struct {
	Access  string    `json:"access"`
	Refresh string    `json:"refresh,omitempty"`
	Expiry  time.Time `json:"-"`
}

// DeviceAuth is a pairing in progress.
type DeviceAuth struct {
	DeviceCode      string
	UserCode        string
	VerificationURL string
	ExpiresIn       time.Duration
	Interval        time.Duration
}

// Account is the remote user.
type Account struct {
	Username string
	ID       string
}

// Creds are the application credentials of a provider.
type Creds struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// Provider is an external service. Methods taking an access token act on
// behalf of a user; the service refreshes tokens before calling them.
type Provider interface {
	Service() string
	Supports(f Feature) bool
	// Handles reports whether a feature covers a kind of item ("movie",
	// "show", "episode"); the merge leaves other kinds alone.
	Handles(f Feature, kind string) bool
	// NeedsSecret reports whether a client secret is part of the app credentials.
	NeedsSecret() bool

	StartAuth(ctx context.Context, c Creds) (DeviceAuth, error)
	// PollAuth returns ErrAuthPending, ErrSlowDown, ErrAuthExpired or
	// ErrAuthDenied until the user approves the code.
	PollAuth(ctx context.Context, c Creds, deviceCode string) (Token, error)
	// Refresh returns ErrUnauthorized when the refresh token is rejected.
	Refresh(ctx context.Context, c Creds, t Token) (Token, error)
	Revoke(ctx context.Context, c Creds, t Token) error
	Account(ctx context.Context, c Creds, access string) (Account, error)

	// Activities returns the latest remote change per feature (zero = unknown).
	Activities(ctx context.Context, c Creds, access string) (map[Feature]time.Time, error)
	// Fetch returns the remote state of each requested feature.
	Fetch(ctx context.Context, c Creds, access string, features []Feature) (map[Feature][]Entry, error)
	// Push adds (or updates) and removes remote entries of a feature.
	Push(ctx context.Context, c Creds, access string, f Feature, add, remove []Entry) error
	// Scrobble reports a playback event; progress is 0-100.
	Scrobble(ctx context.Context, c Creds, access string, a Action, r Ref, progress float64) error
}

var (
	ErrAuthPending  = errors.New("authorization pending")
	ErrSlowDown     = errors.New("slow down")
	ErrAuthExpired  = errors.New("code expired")
	ErrAuthDenied   = errors.New("authorization denied")
	ErrUnauthorized = errors.New("access refused by the service (token revoked or expired)")
	ErrNotSupported = errors.New("not supported by this service")
	ErrNotFound     = errors.New("not found on the service")
)

// RateLimitError is returned when the service asks us to slow down (429 or
// an exhausted X-Ratelimit quota).
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("limite de débit atteinte, réessayer dans %s", e.RetryAfter.Round(time.Second))
}

// RemoteError is an unexpected answer (status, network failure). Messages
// never include credentials or tokens.
type RemoteError struct {
	Status int
	Msg    string
}

func (e *RemoteError) Error() string {
	if e.Status == 0 {
		return "service injoignable : " + e.Msg
	}
	return fmt.Sprintf("réponse inattendue du service (%d) %s", e.Status, e.Msg)
}

// Temporary reports whether retrying later may help.
func (e *RemoteError) Temporary() bool { return e.Status == 0 || e.Status >= 500 || e.Status == 408 }

// flexID decodes a JSON number or string into a string.
type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == "" {
		*f = ""
		return nil
	}
	if s[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flexID(v)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexID(n.String())
	return nil
}

// numID converts a numeric id for providers expecting integers; non-numeric
// ids are left out (0).
func numID(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z", "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
