package syncsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SimklURL is the production API.
const SimklURL = "https://api.simkl.com"

// Simkl implements Provider for simkl.com with AUTH V1 (PIN flow; an AUTH V2
// client_id is refused by /oauth/pin). V1 tokens last about five years, have
// no refresh grant and cannot be revoked through the API. Resume points
// (/sync/playback) are not synchronised yet.
type Simkl struct {
	c       *client
	version string
}

func NewSimkl(base, userAgent, version string, tr http.RoundTripper) *Simkl {
	if base == "" {
		base = SimklURL
	}
	return &Simkl{c: newClient(base, userAgent, 2, 1, tr), version: version}
}

func (*Simkl) Service() string { return "simkl" }
func (*Simkl) Supports(f Feature) bool {
	return f == FeatScrobble || f == FeatHistory || f == FeatWatchlist || f == FeatRatings
}
func (*Simkl) NeedsSecret() bool { return false }

// Handles: Simkl rates movies and shows, not episodes.
func (*Simkl) Handles(f Feature, kind string) bool {
	if f == FeatRatings {
		return kind == "movie" || kind == "show"
	}
	return defaultHandles(f, kind)
}

func (s *Simkl) header(c Creds, access string) http.Header {
	h := http.Header{}
	h.Set("simkl-api-key", c.ClientID)
	if access != "" {
		h.Set("Authorization", "Bearer "+access)
	}
	return h
}

// path appends the identification Simkl asks every client to send.
func (s *Simkl) path(c Creds, p string, extra url.Values) string {
	q := url.Values{}
	for k, v := range extra {
		q[k] = v
	}
	q.Set("client_id", c.ClientID)
	q.Set("app-name", "aether")
	q.Set("app-version", s.version)
	sep := "?"
	if strings.Contains(p, "?") {
		sep = "&"
	}
	return p + sep + q.Encode()
}

func (s *Simkl) StartAuth(ctx context.Context, c Creds) (DeviceAuth, error) {
	var out struct {
		Result          string `json:"result"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		VerificationURL string `json:"verification_url"` // legacy alias
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	res, err := s.c.send(ctx, http.MethodGet, s.path(c, "/oauth/pin", nil), s.header(c, ""), nil)
	if err != nil {
		return DeviceAuth{}, err
	}
	if res.status == http.StatusBadRequest && strings.Contains(string(res.body), "unauthorized_client") {
		return DeviceAuth{}, errAuthV2
	}
	if err := res.decode(http.MethodGet, "/oauth/pin", &out); err != nil {
		return DeviceAuth{}, err
	}
	if out.VerificationURI == "" {
		out.VerificationURI = out.VerificationURL
	}
	if out.UserCode == "" {
		return DeviceAuth{}, &RemoteError{Status: 200, Msg: "code PIN absent"}
	}
	exp := time.Duration(out.ExpiresIn) * time.Second
	if exp <= 0 {
		exp = 15 * time.Minute
	}
	// Simkl polls by user code: it doubles as the device code.
	return DeviceAuth{DeviceCode: out.UserCode, UserCode: out.UserCode, VerificationURL: out.VerificationURI,
		ExpiresIn: exp, Interval: time.Duration(max(out.Interval, 1)) * time.Second}, nil
}

func (s *Simkl) PollAuth(ctx context.Context, c Creds, deviceCode string) (Token, error) {
	res, err := s.c.doReq(ctx, http.MethodGet, s.path(c, "/oauth/pin/"+url.PathEscape(deviceCode), nil), s.header(c, ""), nil, true)
	if err != nil {
		return Token{}, err
	}
	switch res.status {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return Token{}, ErrSlowDown
	case http.StatusNotFound, http.StatusGone:
		return Token{}, ErrAuthExpired
	case http.StatusBadRequest:
		if strings.Contains(string(res.body), "unauthorized_client") {
			return Token{}, errAuthV2
		}
		fallthrough
	default:
		return Token{}, &RemoteError{Status: res.status, Msg: "GET /oauth/pin"}
	}
	var out struct {
		Result      string `json:"result"`
		Message     string `json:"message"`
		AccessToken string `json:"access_token"`
		DeviceCode  string `json:"device_code"`
		UserCode    string `json:"user_code"`
	}
	if json.Unmarshal(res.body, &out) != nil {
		return Token{}, &RemoteError{Status: res.status, Msg: "réponse illisible"}
	}
	if out.Result == "OK" && out.AccessToken != "" {
		return Token{Access: out.AccessToken}, nil
	}
	// An unknown or expired code is answered with a brand-new code (the
	// /oauth/pin shape): the one being polled is gone.
	if out.DeviceCode != "" || out.UserCode != "" {
		return Token{}, ErrAuthExpired
	}
	msg := strings.ToLower(out.Message)
	switch {
	case strings.Contains(msg, "slow"):
		return Token{}, ErrSlowDown
	case strings.Contains(msg, "expire"), strings.Contains(msg, "invalid"):
		return Token{}, ErrAuthExpired
	case strings.Contains(msg, "denied"), strings.Contains(msg, "reject"):
		return Token{}, ErrAuthDenied
	}
	return Token{}, ErrAuthPending
}

func (s *Simkl) Refresh(context.Context, Creds, Token) (Token, error) {
	return Token{}, ErrUnauthorized
}

// Revoke is a no-op: Simkl offers no revocation endpoint.
func (s *Simkl) Revoke(context.Context, Creds, Token) error { return nil }

func (s *Simkl) Account(ctx context.Context, c Creds, access string) (Account, error) {
	var out struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
		Account struct {
			ID flexID `json:"id"`
		} `json:"account"`
	}
	if err := s.c.call(ctx, http.MethodGet, s.path(c, "/users/settings", nil), s.header(c, access), nil, &out); err != nil {
		return Account{}, err
	}
	return Account{Username: out.User.Name, ID: string(out.Account.ID)}, nil
}

func (s *Simkl) Activities(ctx context.Context, c Creds, access string) (map[Feature]time.Time, error) {
	var out struct {
		All string `json:"all"`
	}
	if err := s.c.call(ctx, http.MethodGet, s.path(c, "/sync/activities", nil), s.header(c, access), nil, &out); err != nil {
		return nil, err
	}
	// Simkl reports per-list dates; the global one is enough to skip
	// fetching when nothing moved.
	all := parseTime(out.All)
	return map[Feature]time.Time{FeatHistory: all, FeatWatchlist: all, FeatRatings: all}, nil
}

type simklIDs struct {
	Simkl   flexID `json:"simkl"`
	SimklID flexID `json:"simkl_id"` // same id, under another key on some endpoints
	Tmdb    flexID `json:"tmdb"`
	Imdb    flexID `json:"imdb"`
	Tvdb    flexID `json:"tvdb"`
}

func (i simklIDs) ids() IDs {
	id := i.Simkl
	if id == "" {
		id = i.SimklID
	}
	return IDs{Simkl: string(id), Tmdb: string(i.Tmdb), Imdb: string(i.Imdb), Tvdb: string(i.Tvdb)}
}

// simklTime parses a Simkl date; dates before 2000 are its "watched a long
// time ago, date unknown" placeholder (1970-01-01T00:00:01Z).
func simklTime(v string) time.Time {
	t := parseTime(v)
	if t.Year() < 2000 {
		return time.Time{}
	}
	return t
}

func simklOutIDs(i IDs) map[string]any {
	m := outIDs(IDs{Tmdb: i.Tmdb, Imdb: i.Imdb, Tvdb: i.Tvdb})
	if n := numID(i.Simkl); n > 0 {
		m["simkl"] = n
	}
	return m
}

type simklItem struct {
	Status        string `json:"status"`
	LastWatchedAt string `json:"last_watched_at"`
	AddedToList   string `json:"added_to_watchlist_at"`
	UserRating    float64
	UserRatedAt   string `json:"user_rated_at"`
	Movie         *struct {
		IDs simklIDs `json:"ids"`
	} `json:"movie"`
	Show *struct {
		IDs simklIDs `json:"ids"`
	} `json:"show"`
	Seasons []struct {
		Number   int `json:"number"`
		Episodes []struct {
			Number    int    `json:"number"`
			WatchedAt string `json:"watched_at"`
			// Anime only (extended=full_anime_seasons): the TVDB numbering
			// of this per-cour (AniDB) episode.
			Tvdb *struct {
				Season  int `json:"season"`
				Episode int `json:"episode"`
			} `json:"tvdb"`
		} `json:"episodes"`
	} `json:"seasons"`
}

func (it *simklItem) UnmarshalJSON(b []byte) error {
	type plain simklItem
	var aux struct {
		plain
		UserRating flexID `json:"user_rating"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*it = simklItem(aux.plain)
	it.UserRating = float64(numID(string(aux.UserRating)))
	return nil
}

func (s *Simkl) Fetch(ctx context.Context, c Creds, access string, features []Feature) (map[Feature][]Entry, error) {
	want := map[Feature]bool{}
	for _, f := range features {
		want[f] = true
	}
	out := map[Feature][]Entry{}
	for _, f := range features {
		out[f] = nil
	}
	for _, typ := range []string{"movies", "shows", "anime"} {
		q := url.Values{"extended": {"full"}}
		switch typ {
		case "anime":
			// Per-cour numbering plus its TVDB mapping, which is what the
			// library uses.
			q.Set("extended", "full_anime_seasons")
			fallthrough
		case "shows":
			q.Set("episode_watched_at", "yes")
			// Without it, completed and dropped shows carry no episodes.
			q.Set("include_all_episodes", "yes")
		}
		var res map[string][]simklItem
		if err := s.c.call(ctx, http.MethodGet, s.path(c, "/sync/all-items/"+typ, q), s.header(c, access), nil, &res); err != nil {
			return nil, err
		}
		for _, it := range res[typ] {
			var ref Ref
			switch {
			case it.Movie != nil:
				ref = Ref{Kind: "movie", IDs: it.Movie.IDs.ids()}
			case it.Show != nil:
				ref = Ref{Kind: "show", IDs: it.Show.IDs.ids()}
			default:
				continue
			}
			if want[FeatHistory] {
				if ref.Kind == "movie" && it.Status == "completed" {
					out[FeatHistory] = append(out[FeatHistory], Entry{Ref: ref, At: simklTime(it.LastWatchedAt)})
				}
				for _, se := range it.Seasons {
					for _, e := range se.Episodes {
						season, number := se.Number, e.Number
						if typ == "anime" {
							if e.Tvdb == nil {
								continue // no TVDB mapping: cannot be placed in the library
							}
							season, number = e.Tvdb.Season, e.Tvdb.Episode
						}
						out[FeatHistory] = append(out[FeatHistory], Entry{
							Ref: Ref{Kind: "episode", IDs: ref.IDs, Season: season, Episode: number}, At: simklTime(e.WatchedAt)})
					}
				}
			}
			if want[FeatWatchlist] && it.Status == "plantowatch" {
				out[FeatWatchlist] = append(out[FeatWatchlist], Entry{Ref: ref, At: simklTime(it.AddedToList)})
			}
			if want[FeatRatings] && it.UserRating > 0 {
				out[FeatRatings] = append(out[FeatRatings], Entry{Ref: ref, At: parseTime(it.UserRatedAt), Value: it.UserRating})
			}
		}
	}
	return out, nil
}

func (s *Simkl) Push(ctx context.Context, c Creds, access string, f Feature, add, remove []Entry) error {
	h := s.header(c, access)
	post := func(p string, entries []Entry, timeKey, valueKey string, extra map[string]any) error {
		for start := 0; start < len(entries); start += pushChunk {
			body := groupBody(entries[start:min(start+pushChunk, len(entries))], simklOutIDs, timeKey, valueKey)
			for _, k := range []string{"movies", "shows"} {
				if list, ok := body[k].([]map[string]any); ok {
					for _, m := range list {
						for ek, ev := range extra {
							m[ek] = ev
						}
					}
				}
			}
			if err := s.c.call(ctx, http.MethodPost, s.path(c, p, nil), h, body, nil); err != nil {
				return err
			}
		}
		return nil
	}
	switch f {
	case FeatHistory:
		if err := post("/sync/history", add, "watched_at", "", nil); err != nil {
			return err
		}
		return post("/sync/history/remove", remove, "", "", nil)
	case FeatWatchlist:
		if err := post("/sync/add-to-list", add, "added_at", "", map[string]any{"to": "plantowatch"}); err != nil {
			return err
		}
		// Removing from a list goes through history/remove; only items in
		// "plan to watch" (never watched) are ever removed this way.
		return post("/sync/history/remove", remove, "", "", nil)
	case FeatRatings:
		if err := post("/sync/ratings", add, "rated_at", "rating", nil); err != nil {
			return err
		}
		return post("/sync/ratings/remove", remove, "", "", nil)
	}
	return ErrNotSupported
}

// Scrobble sends a live playback event (/scrobble/start|pause|stop, same
// body as Trakt's). A stop at 80 % or more marks the item watched.
func (s *Simkl) Scrobble(ctx context.Context, c Creds, access string, a Action, r Ref, progress float64) error {
	body := map[string]any{"progress": math.Round(min(max(progress, 0), 100)*100) / 100}
	switch r.Kind {
	case "movie":
		body["movie"] = map[string]any{"ids": simklOutIDs(r.IDs)}
	case "episode":
		body["show"] = map[string]any{"ids": simklOutIDs(r.IDs)}
		body["episode"] = map[string]any{"season": r.Season, "number": r.Episode}
	default:
		return ErrNotSupported
	}
	p := "/scrobble/" + string(a)
	res, err := s.c.send(ctx, http.MethodPost, s.path(c, p, nil), s.header(c, access), body)
	if err != nil {
		return err
	}
	if res.status == http.StatusConflict {
		return nil // stopped again within the hour: already recorded
	}
	if res.status == http.StatusNotFound {
		return fmt.Errorf("%w: élément inconnu de Simkl", ErrNotFound)
	}
	return res.decode(http.MethodPost, p, nil)
}

// errAuthV2 explains a client_id registered for Simkl AUTH V2, which the PIN
// flow (AUTH V1) refuses.
var errAuthV2 = &RemoteError{Status: http.StatusBadRequest,
	Msg: "ce client_id Simkl est une application AUTH V2 : Aether utilise le flux PIN (AUTH V1), créez une application V1"}
