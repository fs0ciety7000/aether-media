package syncsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// TraktURL is the production API.
const TraktURL = "https://api.trakt.tv"

// Trakt implements Provider for trakt.tv (API v2).
type Trakt struct{ c *client }

// NewTrakt builds a client for base (TraktURL in production). Writes are
// spaced by one second (Trakt's limit for authenticated POST/PUT/DELETE).
func NewTrakt(base, userAgent string, tr http.RoundTripper) *Trakt {
	if base == "" {
		base = TraktURL
	}
	return &Trakt{c: newClient(base, userAgent, 3, 1, tr)}
}

func (*Trakt) Service() string       { return "trakt" }
func (*Trakt) Supports(Feature) bool { return true }
func (*Trakt) NeedsSecret() bool     { return true }

func (*Trakt) Handles(f Feature, kind string) bool { return defaultHandles(f, kind) }

func (t *Trakt) header(c Creds, access string) http.Header {
	h := http.Header{}
	h.Set("trakt-api-version", "2")
	h.Set("trakt-api-key", c.ClientID)
	if access != "" {
		h.Set("Authorization", "Bearer "+access)
	}
	return h
}

// ---- OAuth ----

func (t *Trakt) StartAuth(ctx context.Context, c Creds) (DeviceAuth, error) {
	var out struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := t.c.call(ctx, http.MethodPost, "/oauth/device/code", t.header(c, ""), map[string]string{"client_id": c.ClientID}, &out); err != nil {
		return DeviceAuth{}, err
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return DeviceAuth{}, &RemoteError{Status: 200, Msg: "code d'appareil absent"}
	}
	return DeviceAuth{DeviceCode: out.DeviceCode, UserCode: out.UserCode, VerificationURL: out.VerificationURL,
		ExpiresIn: time.Duration(out.ExpiresIn) * time.Second, Interval: time.Duration(max(out.Interval, 1)) * time.Second}, nil
}

type traktToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	CreatedAt    int64  `json:"created_at"`
}

func (tt traktToken) token(now time.Time) Token {
	created := now
	if tt.CreatedAt > 0 {
		created = time.Unix(tt.CreatedAt, 0)
	}
	tok := Token{Access: tt.AccessToken, Refresh: tt.RefreshToken}
	if tt.ExpiresIn > 0 {
		tok.Expiry = created.Add(time.Duration(tt.ExpiresIn) * time.Second)
	}
	return tok
}

func (t *Trakt) PollAuth(ctx context.Context, c Creds, deviceCode string) (Token, error) {
	res, err := t.c.doReq(ctx, http.MethodPost, "/oauth/device/token", t.header(c, ""),
		map[string]string{"code": deviceCode, "client_id": c.ClientID, "client_secret": c.ClientSecret}, true)
	if err != nil {
		return Token{}, err
	}
	switch res.status {
	case http.StatusOK:
		var tt traktToken
		if json.Unmarshal(res.body, &tt) != nil || tt.AccessToken == "" {
			return Token{}, &RemoteError{Status: res.status, Msg: "jeton illisible"}
		}
		return tt.token(t.c.now()), nil
	case http.StatusBadRequest:
		return Token{}, ErrAuthPending
	case http.StatusTooManyRequests:
		return Token{}, ErrSlowDown
	case http.StatusNotFound, http.StatusConflict, http.StatusGone:
		return Token{}, ErrAuthExpired
	case 418:
		return Token{}, ErrAuthDenied
	}
	return Token{}, &RemoteError{Status: res.status, Msg: "POST /oauth/device/token"}
}

func (t *Trakt) Refresh(ctx context.Context, c Creds, tok Token) (Token, error) {
	if tok.Refresh == "" {
		return Token{}, ErrUnauthorized
	}
	res, err := t.c.send(ctx, http.MethodPost, "/oauth/token", t.header(c, ""), map[string]string{
		"refresh_token": tok.Refresh, "client_id": c.ClientID, "client_secret": c.ClientSecret,
		"redirect_uri": "urn:ietf:wg:oauth:2.0:oob", "grant_type": "refresh_token",
	})
	if err != nil {
		return Token{}, err
	}
	switch res.status {
	case http.StatusOK:
		var tt traktToken
		if json.Unmarshal(res.body, &tt) != nil || tt.AccessToken == "" {
			return Token{}, &RemoteError{Status: res.status, Msg: "jeton illisible"}
		}
		return tt.token(t.c.now()), nil
	case http.StatusBadRequest, http.StatusUnauthorized:
		return Token{}, ErrUnauthorized // invalid_grant: the user must reconnect
	}
	return Token{}, &RemoteError{Status: res.status, Msg: "POST /oauth/token"}
}

func (t *Trakt) Revoke(ctx context.Context, c Creds, tok Token) error {
	return t.c.call(ctx, http.MethodPost, "/oauth/revoke", t.header(c, ""),
		map[string]string{"token": tok.Access, "client_id": c.ClientID, "client_secret": c.ClientSecret}, nil)
}

func (t *Trakt) Account(ctx context.Context, c Creds, access string) (Account, error) {
	var out struct {
		User struct {
			Username string `json:"username"`
			IDs      struct {
				Slug string `json:"slug"`
			} `json:"ids"`
		} `json:"user"`
	}
	if err := t.c.call(ctx, http.MethodGet, "/users/settings", t.header(c, access), nil, &out); err != nil {
		return Account{}, err
	}
	return Account{Username: out.User.Username, ID: out.User.IDs.Slug}, nil
}

// ---- data ----

type traktIDs struct {
	Trakt flexID `json:"trakt,omitempty"`
	Tmdb  flexID `json:"tmdb,omitempty"`
	Imdb  flexID `json:"imdb,omitempty"`
	Tvdb  flexID `json:"tvdb,omitempty"`
}

func (i traktIDs) ids() IDs {
	return IDs{Trakt: string(i.Trakt), Tmdb: string(i.Tmdb), Imdb: string(i.Imdb), Tvdb: string(i.Tvdb)}
}

// outIDs renders IDs the way Trakt expects them (integers except imdb).
func outIDs(i IDs) map[string]any {
	m := map[string]any{}
	if n := numID(i.Trakt); n > 0 {
		m["trakt"] = n
	}
	if n := numID(i.Tmdb); n > 0 {
		m["tmdb"] = n
	}
	if i.Imdb != "" {
		m["imdb"] = i.Imdb
	}
	if n := numID(i.Tvdb); n > 0 {
		m["tvdb"] = n
	}
	return m
}

type traktMedia struct {
	IDs    traktIDs `json:"ids"`
	Season int      `json:"season"`
	Number int      `json:"number"`
}

func (t *Trakt) Activities(ctx context.Context, c Creds, access string) (map[Feature]time.Time, error) {
	var out map[string]json.RawMessage
	if err := t.c.call(ctx, http.MethodGet, "/sync/last_activities", t.header(c, access), nil, &out); err != nil {
		return nil, err
	}
	field := func(group, name string) time.Time {
		var g map[string]string
		if raw, ok := out[group]; !ok || json.Unmarshal(raw, &g) != nil {
			return time.Time{}
		}
		return parseTime(g[name])
	}
	latest := func(ts ...time.Time) time.Time {
		var m time.Time
		for _, t := range ts {
			if t.After(m) {
				m = t
			}
		}
		return m
	}
	return map[Feature]time.Time{
		FeatHistory:   latest(field("movies", "watched_at"), field("episodes", "watched_at")),
		FeatProgress:  latest(field("movies", "paused_at"), field("episodes", "paused_at")),
		FeatWatchlist: latest(field("movies", "watchlisted_at"), field("shows", "watchlisted_at")),
		FeatRatings:   latest(field("movies", "rated_at"), field("shows", "rated_at"), field("episodes", "rated_at")),
	}, nil
}

func (t *Trakt) Fetch(ctx context.Context, c Creds, access string, features []Feature) (map[Feature][]Entry, error) {
	out := map[Feature][]Entry{}
	h := t.header(c, access)
	get := func(path string, v any) error { return t.c.call(ctx, http.MethodGet, path, h, nil, v) }
	for _, f := range features {
		var entries []Entry
		switch f {
		case FeatHistory:
			var movies []struct {
				LastWatchedAt string     `json:"last_watched_at"`
				Movie         traktMedia `json:"movie"`
			}
			if err := get("/sync/watched/movies", &movies); err != nil {
				return nil, err
			}
			for _, m := range movies {
				entries = append(entries, Entry{Ref: Ref{Kind: "movie", IDs: m.Movie.IDs.ids()}, At: parseTime(m.LastWatchedAt)})
			}
			var shows []struct {
				Show    traktMedia `json:"show"`
				Seasons []struct {
					Number   int `json:"number"`
					Episodes []struct {
						Number        int    `json:"number"`
						LastWatchedAt string `json:"last_watched_at"`
					} `json:"episodes"`
				} `json:"seasons"`
			}
			if err := get("/sync/watched/shows", &shows); err != nil {
				return nil, err
			}
			for _, s := range shows {
				for _, se := range s.Seasons {
					for _, e := range se.Episodes {
						entries = append(entries, Entry{Ref: Ref{Kind: "episode", IDs: s.Show.IDs.ids(), Season: se.Number, Episode: e.Number},
							At: parseTime(e.LastWatchedAt)})
					}
				}
			}
		case FeatProgress:
			for _, path := range []string{"/sync/playback/movies", "/sync/playback/episodes"} {
				var rows []struct {
					ID       flexID     `json:"id"`
					Progress float64    `json:"progress"`
					PausedAt string     `json:"paused_at"`
					Type     string     `json:"type"`
					Movie    traktMedia `json:"movie"`
					Episode  traktMedia `json:"episode"`
					Show     traktMedia `json:"show"`
				}
				if err := get(path, &rows); err != nil {
					return nil, err
				}
				for _, r := range rows {
					e := Entry{At: parseTime(r.PausedAt), Value: r.Progress, RemoteRef: string(r.ID)}
					if r.Type == "movie" {
						e.Ref = Ref{Kind: "movie", IDs: r.Movie.IDs.ids()}
					} else {
						e.Ref = Ref{Kind: "episode", IDs: r.Show.IDs.ids(), Season: r.Episode.Season, Episode: r.Episode.Number}
					}
					entries = append(entries, e)
				}
			}
		case FeatWatchlist:
			for _, path := range []string{"/sync/watchlist/movies", "/sync/watchlist/shows"} {
				var rows []struct {
					ListedAt string     `json:"listed_at"`
					Type     string     `json:"type"`
					Movie    traktMedia `json:"movie"`
					Show     traktMedia `json:"show"`
				}
				if err := get(path, &rows); err != nil {
					return nil, err
				}
				for _, r := range rows {
					switch r.Type {
					case "movie":
						entries = append(entries, Entry{Ref: Ref{Kind: "movie", IDs: r.Movie.IDs.ids()}, At: parseTime(r.ListedAt)})
					case "show":
						entries = append(entries, Entry{Ref: Ref{Kind: "show", IDs: r.Show.IDs.ids()}, At: parseTime(r.ListedAt)})
					}
				}
			}
		case FeatRatings:
			for _, path := range []string{"/sync/ratings/movies", "/sync/ratings/shows", "/sync/ratings/episodes"} {
				var rows []struct {
					RatedAt string     `json:"rated_at"`
					Rating  float64    `json:"rating"`
					Type    string     `json:"type"`
					Movie   traktMedia `json:"movie"`
					Show    traktMedia `json:"show"`
					Episode traktMedia `json:"episode"`
				}
				if err := get(path, &rows); err != nil {
					return nil, err
				}
				for _, r := range rows {
					e := Entry{At: parseTime(r.RatedAt), Value: r.Rating}
					switch r.Type {
					case "movie":
						e.Ref = Ref{Kind: "movie", IDs: r.Movie.IDs.ids()}
					case "show":
						e.Ref = Ref{Kind: "show", IDs: r.Show.IDs.ids()}
					case "episode":
						e.Ref = Ref{Kind: "episode", IDs: r.Show.IDs.ids(), Season: r.Episode.Season, Episode: r.Episode.Number}
					default:
						continue
					}
					entries = append(entries, e)
				}
			}
		}
		out[f] = entries
	}
	return out, nil
}

const pushChunk = 100

func (t *Trakt) Push(ctx context.Context, c Creds, access string, f Feature, add, remove []Entry) error {
	h := t.header(c, access)
	post := func(path string, entries []Entry, withTime string, withValue string) error {
		for start := 0; start < len(entries); start += pushChunk {
			body := groupBody(entries[start:min(start+pushChunk, len(entries))], outIDs, withTime, withValue)
			if err := t.c.call(ctx, http.MethodPost, path, h, body, nil); err != nil {
				return err
			}
		}
		return nil
	}
	switch f {
	case FeatHistory:
		if err := post("/sync/history", add, "watched_at", ""); err != nil {
			return err
		}
		return post("/sync/history/remove", remove, "", "")
	case FeatWatchlist:
		if err := post("/sync/watchlist", add, "listed_at", ""); err != nil {
			return err
		}
		return post("/sync/watchlist/remove", remove, "", "")
	case FeatRatings:
		if err := post("/sync/ratings", add, "rated_at", "rating"); err != nil {
			return err
		}
		return post("/sync/ratings/remove", remove, "", "")
	case FeatProgress:
		// Trakt has no batch endpoint: a resume point is a paused scrobble.
		for _, e := range add {
			if err := t.Scrobble(ctx, c, access, ActPause, e.Ref, e.Value); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
		for _, e := range remove {
			if e.RemoteRef == "" {
				continue
			}
			err := t.c.call(ctx, http.MethodDelete, "/sync/playback/"+e.RemoteRef, h, nil, nil)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
		return nil
	}
	return ErrNotSupported
}

// groupBody groups entries into {movies, shows[seasons[episodes]]}, the
// shape shared by Trakt and Simkl sync endpoints.
func groupBody(entries []Entry, outIDs func(IDs) map[string]any, timeKey, valueKey string) map[string]any {
	decorate := func(m map[string]any, e Entry) map[string]any {
		if timeKey != "" && !e.At.IsZero() {
			m[timeKey] = e.At.UTC().Format(time.RFC3339)
		}
		if valueKey != "" {
			m[valueKey] = int(e.Value)
		}
		return m
	}
	var movies, shows []map[string]any
	type season map[int][]map[string]any
	episodes := map[string]season{}
	showIDs := map[string]IDs{}
	var order []string
	for _, e := range entries {
		switch e.Kind {
		case "movie":
			movies = append(movies, decorate(map[string]any{"ids": outIDs(e.IDs)}, e))
		case "show":
			shows = append(shows, decorate(map[string]any{"ids": outIDs(e.IDs)}, e))
		case "episode":
			k := fmt.Sprint(outIDs(e.IDs))
			if _, ok := episodes[k]; !ok {
				episodes[k] = season{}
				showIDs[k] = e.IDs
				order = append(order, k)
			}
			episodes[k][e.Season] = append(episodes[k][e.Season], decorate(map[string]any{"number": e.Episode}, e))
		}
	}
	for _, k := range order {
		var seasons []map[string]any
		nums := make([]int, 0, len(episodes[k]))
		for n := range episodes[k] {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		for _, n := range nums {
			seasons = append(seasons, map[string]any{"number": n, "episodes": episodes[k][n]})
		}
		shows = append(shows, map[string]any{"ids": outIDs(showIDs[k]), "seasons": seasons})
	}
	body := map[string]any{}
	if len(movies) > 0 {
		body["movies"] = movies
	}
	if len(shows) > 0 {
		body["shows"] = shows
	}
	return body
}

func (t *Trakt) Scrobble(ctx context.Context, c Creds, access string, a Action, r Ref, progress float64) error {
	body := map[string]any{"progress": min(max(progress, 0), 100)}
	switch r.Kind {
	case "movie":
		body["movie"] = map[string]any{"ids": outIDs(r.IDs)}
	case "episode":
		body["show"] = map[string]any{"ids": outIDs(r.IDs)}
		body["episode"] = map[string]any{"season": r.Season, "number": r.Episode}
	default:
		return ErrNotSupported
	}
	res, err := t.c.send(ctx, http.MethodPost, "/scrobble/"+string(a), t.header(c, access), body)
	if err != nil {
		return err
	}
	switch {
	case res.status >= 200 && res.status < 300:
		return nil
	case res.status == http.StatusConflict:
		return nil // already scrobbled a moment ago
	case res.status == http.StatusUnauthorized:
		return ErrUnauthorized
	case res.status == http.StatusNotFound:
		return fmt.Errorf("%w: élément inconnu de Trakt", ErrNotFound)
	}
	return &RemoteError{Status: res.status, Msg: "POST /scrobble/" + string(a)}
}
