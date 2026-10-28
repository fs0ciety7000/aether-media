package syncsvc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SimklURL is the production API.
const SimklURL = "https://api.simkl.com"

// Simkl implements Provider for simkl.com. Simkl has no playback progress
// API we rely on: "scrobbling" adds the item to the history when it ends.
// Its tokens do not expire and cannot be revoked through the API.
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
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := s.c.call(ctx, http.MethodGet, s.path(c, "/oauth/pin", url.Values{"redirect": {""}}), s.header(c, ""), nil, &out); err != nil {
		return DeviceAuth{}, err
	}
	if out.UserCode == "" {
		return DeviceAuth{}, &RemoteError{Status: 200, Msg: "code PIN absent"}
	}
	exp := time.Duration(out.ExpiresIn) * time.Second
	if exp <= 0 {
		exp = 15 * time.Minute
	}
	// Simkl polls by user code: it doubles as the device code.
	return DeviceAuth{DeviceCode: out.UserCode, UserCode: out.UserCode, VerificationURL: out.VerificationURL,
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
	default:
		return Token{}, &RemoteError{Status: res.status, Msg: "GET /oauth/pin"}
	}
	var out struct {
		Result      string `json:"result"`
		Message     string `json:"message"`
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(res.body, &out) != nil {
		return Token{}, &RemoteError{Status: res.status, Msg: "réponse illisible"}
	}
	if out.Result == "OK" && out.AccessToken != "" {
		return Token{Access: out.AccessToken}, nil
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
	if err := s.c.call(ctx, http.MethodPost, s.path(c, "/users/settings", nil), s.header(c, access), map[string]any{}, &out); err != nil {
		return Account{}, err
	}
	return Account{Username: out.User.Name, ID: string(out.Account.ID)}, nil
}

func (s *Simkl) Activities(ctx context.Context, c Creds, access string) (map[Feature]time.Time, error) {
	var out struct {
		All string `json:"all"`
	}
	if err := s.c.call(ctx, http.MethodPost, s.path(c, "/sync/activities", nil), s.header(c, access), map[string]any{}, &out); err != nil {
		return nil, err
	}
	// Simkl reports per-list dates; the global one is enough to skip
	// fetching when nothing moved.
	all := parseTime(out.All)
	return map[Feature]time.Time{FeatHistory: all, FeatWatchlist: all, FeatRatings: all}, nil
}

type simklIDs struct {
	Simkl flexID `json:"simkl"`
	Tmdb  flexID `json:"tmdb"`
	Imdb  flexID `json:"imdb"`
	Tvdb  flexID `json:"tvdb"`
}

func (i simklIDs) ids() IDs {
	return IDs{Simkl: string(i.Simkl), Tmdb: string(i.Tmdb), Imdb: string(i.Imdb), Tvdb: string(i.Tvdb)}
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
		if typ != "movies" {
			q.Set("episode_watched_at", "yes")
		}
		var res map[string][]simklItem
		if err := s.c.call(ctx, http.MethodGet, s.path(c, "/sync/all-items/"+typ+"/", q), s.header(c, access), nil, &res); err != nil {
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
					out[FeatHistory] = append(out[FeatHistory], Entry{Ref: ref, At: parseTime(it.LastWatchedAt)})
				}
				for _, se := range it.Seasons {
					for _, e := range se.Episodes {
						out[FeatHistory] = append(out[FeatHistory], Entry{
							Ref: Ref{Kind: "episode", IDs: ref.IDs, Season: se.Number, Episode: e.Number}, At: parseTime(e.WatchedAt)})
					}
				}
			}
			if want[FeatWatchlist] && it.Status == "plantowatch" {
				out[FeatWatchlist] = append(out[FeatWatchlist], Entry{Ref: ref, At: parseTime(it.AddedToList)})
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
		if err := post("/sync/add-to-list", add, "", "", map[string]any{"to": "plantowatch"}); err != nil {
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

// Scrobble records the item as watched once playback ends; Simkl gets no
// live start/pause events.
func (s *Simkl) Scrobble(ctx context.Context, c Creds, access string, a Action, r Ref, progress float64) error {
	if a != ActStop || progress < 80 {
		return nil
	}
	return s.Push(ctx, c, access, FeatHistory, []Entry{{Ref: r, At: s.c.now()}}, nil)
}
