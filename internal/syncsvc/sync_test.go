package syncsvc

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

func TestTraktDeviceCode(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	e.ft.json("POST", "/oauth/device/code", 200, `{"device_code":"dev-secret","user_code":"AB12CD34",
		"verification_url":"https://trakt.tv/activate","expires_in":600,"interval":5}`)
	statuses := []int{400, 429, 200}
	e.ft.on("POST", "/oauth/device/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["code"] != "dev-secret" || body["client_id"] != "cid-trakt" || body["client_secret"] != "csecret-trakt" {
			t.Errorf("poll body: %v", body)
		}
		st := statuses[0]
		statuses = statuses[1:]
		w.WriteHeader(st)
		if st == 200 {
			_, _ = w.Write([]byte(`{"access_token":"tok-access","refresh_token":"tok-refresh","expires_in":86400,"created_at":` +
				itoa(e.clock.Unix()) + `}`))
		}
	})

	pend, err := e.svc.Connect(e.ctx, e.user, "trakt")
	if err != nil {
		t.Fatal(err)
	}
	if pend.UserCode != "AB12CD34" || pend.VerificationURL != "https://trakt.tv/activate" || pend.Interval != 5 {
		t.Fatalf("pending: %+v", pend)
	}
	c, _ := e.svc.Connection(e.ctx, e.user, "trakt")
	if c.Status != "pending" || c.Pending == nil {
		t.Fatalf("status after connect: %+v", c)
	}

	res, err := e.svc.Poll(e.ctx, e.user, "trakt")
	if err != nil || res.Status != "pending" {
		t.Fatalf("poll 1: %+v %v", res, err)
	}
	// Too soon: the service is not asked again.
	if res, _ = e.svc.Poll(e.ctx, e.user, "trakt"); res.Status != "pending" || e.ft.count("POST", "/oauth/device/token") != 1 {
		t.Fatalf("poll too soon: %+v, %d calls", res, e.ft.count("POST", "/oauth/device/token"))
	}
	e.clock = e.clock.Add(6 * time.Second)
	if res, _ = e.svc.Poll(e.ctx, e.user, "trakt"); res.Status != "slow_down" || res.Interval != 10 {
		t.Fatalf("poll slow_down: %+v", res)
	}
	e.clock = e.clock.Add(6 * time.Second) // < new interval
	if res, _ = e.svc.Poll(e.ctx, e.user, "trakt"); res.Status != "pending" || e.ft.count("POST", "/oauth/device/token") != 2 {
		t.Fatalf("slow_down respected: %+v", res)
	}
	e.clock = e.clock.Add(5 * time.Second)
	res, err = e.svc.Poll(e.ctx, e.user, "trakt")
	if err != nil || res.Status != "connected" {
		t.Fatalf("poll success: %+v %v", res, err)
	}
	if res.Connection.Status != "connected" || res.Connection.Account == nil || res.Connection.Account.Username != "alice_t" ||
		!res.Connection.Syncing || !res.Connection.Features[FeatHistory] {
		t.Fatalf("connection: %+v", res.Connection)
	}
	// Tokens never leave the service, and are encrypted at rest.
	b, _ := json.Marshal(res)
	for _, secret := range []string{"tok-access", "tok-refresh", "dev-secret"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("%s leaked in %s", secret, b)
		}
	}
	conn, _ := e.st.GetSyncConnection(e.ctx, db.GetSyncConnectionParams{UserID: e.user, Service: "trakt"})
	if bytes.Contains(conn.Secret, []byte("tok-access")) || conn.PendingSecret != nil {
		t.Error("token stored in clear or pairing not cleared")
	}
	if _, err := e.svc.Poll(e.ctx, e.user, "trakt"); !errors.Is(err, ErrNotPending) {
		t.Errorf("poll after success: %v", err)
	}
	// Initial import queued, then runs fine against the (empty) remote.
	if errs := e.drain(JobRun); len(errs) > 0 {
		t.Fatal(errs)
	}
	if got := e.ft.find("GET", "/sync/watched/movies"); len(got) != 1 || got[0].auth != "Bearer tok-access" {
		t.Errorf("initial import calls: %+v", got)
	}
}

func TestTraktDeviceCodeExpired(t *testing.T) {
	e := newEnv(t)
	e.ft.json("POST", "/oauth/device/code", 200, `{"device_code":"d","user_code":"U","verification_url":"v","expires_in":30,"interval":5}`)
	e.ft.json("POST", "/oauth/device/token", 410, ``)
	if _, err := e.svc.Connect(e.ctx, e.user, "trakt"); err != nil {
		t.Fatal(err)
	}
	if res, _ := e.svc.Poll(e.ctx, e.user, "trakt"); res.Status != "expired" || res.Connection.Status != "disconnected" {
		t.Fatalf("410: %+v", res)
	}
	// Local expiry, without asking the service.
	if _, err := e.svc.Connect(e.ctx, e.user, "trakt"); err != nil {
		t.Fatal(err)
	}
	e.clock = e.clock.Add(31 * time.Second)
	calls := e.ft.count("POST", "/oauth/device/token")
	if res, _ := e.svc.Poll(e.ctx, e.user, "trakt"); res.Status != "expired" || e.ft.count("POST", "/oauth/device/token") != calls {
		t.Fatalf("local expiry: %+v", res)
	}
	// Unconfigured service.
	e.svc.opts.Config = nil
	if _, err := e.svc.Connect(e.ctx, e.user, "trakt"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("unconfigured: %v", err)
	}
}

func TestTokenRefresh(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	refreshes := 0
	e.ft.on("POST", "/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		refreshes++
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "r1" || body["client_secret"] != "csecret-trakt" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"a2","refresh_token":"r2","expires_in":86400}`))
	})
	// Expires in 10 minutes: refreshed before use.
	c := e.connect(e.trakt, Token{Access: "a1", Refresh: "r1", Expiry: e.clock.Add(10 * time.Minute)})
	if errs := e.drain(JobRun); len(errs) > 0 {
		t.Fatal(errs)
	}
	if refreshes != 1 {
		t.Fatalf("refreshes = %d", refreshes)
	}
	if got := e.ft.find("GET", "/sync/last_activities"); len(got) == 0 || got[0].auth != "Bearer a2" {
		t.Fatalf("calls after refresh: %+v", got)
	}
	conn, _ := e.st.GetSyncConnectionByID(e.ctx, c.ID)
	var tok Token
	if err := e.box.OpenJSON(conn.Secret, &tok); err != nil || tok.Access != "a2" || tok.Refresh != "r2" {
		t.Fatalf("stored token: %+v %v", tok, err)
	}
	if !conn.TokenExpiresAt.Valid || conn.TokenExpiresAt.Int64 < e.clock.Add(23*time.Hour).Unix() {
		t.Errorf("expiry: %+v", conn.TokenExpiresAt)
	}

	// A 401 triggers one refresh and a retry; a refused refresh asks to reconnect.
	e.ft.on("GET", "/sync/last_activities", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) })
	if _, err := e.svc.RunNow(e.ctx, e.user, "trakt", false); err != nil {
		t.Fatal(err)
	}
	errs := e.drain(JobRun)
	if len(errs) != 1 || !errors.Is(errs[0], ErrReauth) || !errors.Is(errs[0], jobs.ErrPermanent) {
		t.Fatalf("errors: %v", errs)
	}
	if refreshes != 2 { // r2 refused (the fake only accepts r1)
		t.Errorf("refreshes = %d", refreshes)
	}
	v, _ := e.svc.Connection(e.ctx, e.user, "trakt")
	if v.Status != "reauth_required" || v.LastError == "" {
		t.Errorf("status: %+v", v)
	}
	if _, err := e.svc.RunNow(e.ctx, e.user, "trakt", false); !errors.Is(err, ErrReauth) {
		t.Errorf("run when reauth required: %v", err)
	}
}

func TestScrobbleStartStop(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	movie := e.movie("Heat", `{"tmdb":"949","imdb":"tt0113277"}`)
	series := e.item("series", "Breaking Bad", `{"tvdb":"81189","tmdb":"1396"}`, 0, 0, 0)
	ep := e.episode(series, 1, 2)
	c := e.connect(e.trakt, Token{Access: "a1"})
	e.drain(JobRun)

	e.svc.Scrobble(e.ctx, e.user, movie, ActStart, 12.5)
	e.svc.Scrobble(e.ctx, e.user, ep, ActStop, 95)
	if errs := e.drain(JobScrobble); len(errs) > 0 {
		t.Fatal(errs)
	}
	start := e.ft.find("POST", "/scrobble/start")
	if len(start) != 1 {
		t.Fatalf("start calls: %+v", start)
	}
	var sb map[string]any
	_ = json.Unmarshal([]byte(start[0].body), &sb)
	ids := sb["movie"].(map[string]any)["ids"].(map[string]any)
	if ids["tmdb"] != float64(949) || ids["imdb"] != "tt0113277" || sb["progress"] != 12.5 {
		t.Errorf("start body: %s", start[0].body)
	}
	stop := e.ft.find("POST", "/scrobble/stop")
	if len(stop) != 1 || !strings.Contains(stop[0].body, `"tvdb":81189`) || !strings.Contains(stop[0].body, `"episode":{"number":2,"season":1}`) {
		t.Fatalf("stop: %+v", stop)
	}
	// The play is recorded by Trakt: the next merge does not push it again.
	rows, _ := e.st.ListSyncState(e.ctx, db.ListSyncStateParams{ConnectionID: c.ID, Feature: "history"})
	if len(rows) != 1 || rows[0].ItemID != ep {
		t.Errorf("history shadow: %+v", rows)
	}
	if err := e.st.MarkPlayed(e.ctx, db.MarkPlayedParams{UserID: e.user, ItemID: ep}); err != nil {
		t.Fatal(err)
	}
	e.ft.json("GET", "/sync/watched/shows", 200, `[{"show":{"ids":{"tvdb":81189}},"seasons":[{"number":1,"episodes":[{"number":2,"last_watched_at":"2026-10-28T10:00:00.000Z"}]}]}]`)
	e.svc.LocalChanged(e.ctx, e.user)
	e.drain(JobRun)
	if n := e.ft.count("POST", "/sync/history"); n != 0 {
		t.Errorf("history pushed %d times after scrobble", n)
	}
	// Disabled scrobbling queues nothing; stale "start" events are dropped.
	if _, err := e.svc.SetFeatures(e.ctx, e.user, "trakt", map[Feature]bool{FeatScrobble: false}); err != nil {
		t.Fatal(err)
	}
	e.svc.Scrobble(e.ctx, e.user, movie, ActPause, 50)
	e.drain(JobScrobble)
	if e.ft.count("POST", "/scrobble/pause") != 0 {
		t.Error("scrobble sent while disabled")
	}
}

func TestHistoryImportAndMatching(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	heat := e.movie("Heat", `{"imdb":"tt0113277"}`) // matched by imdb
	alien := e.movie("Alien", `{"tmdb":"348"}`)     // local only → pushed
	_ = e.movie("Sans identifiant", `{}`)           // never pushed
	series := e.item("series", "BB", `{"tvdb":"81189"}`, 0, 0, 0)
	e1 := e.episode(series, 1, 1)
	e2 := e.episode(series, 1, 2)
	// A double episode file S02E01-E02.
	e21 := e.episode(series, 2, 1)
	e.exec(`UPDATE items SET index_number_end = 2 WHERE id = ?`, e21)
	if err := e.st.MarkWatched(e.ctx, db.MarkWatchedParams{UserID: e.user, ItemID: alien}); err != nil {
		t.Fatal(err)
	}
	e.ft.json("GET", "/sync/watched/movies", 200, `[
		{"plays":1,"last_watched_at":"2026-10-20T20:00:00.000Z","movie":{"title":"Heat","ids":{"trakt":1,"imdb":"tt0113277","tmdb":949}}},
		{"plays":1,"last_watched_at":"2026-10-20T20:00:00.000Z","movie":{"title":"Absent","ids":{"trakt":2,"tmdb":999999}}}]`)
	e.ft.json("GET", "/sync/watched/shows", 200, `[{"show":{"title":"BB","ids":{"trakt":3,"tvdb":81189,"tmdb":1396}},"seasons":[
		{"number":1,"episodes":[{"number":2,"plays":1,"last_watched_at":"2026-10-21T20:00:00.000Z"}]},
		{"number":2,"episodes":[{"number":2,"plays":1,"last_watched_at":"2026-10-22T20:00:00.000Z"}]}]}]`)
	c := e.connect(e.trakt, Token{Access: "a1"})
	if errs := e.drain(JobRun); len(errs) > 0 {
		t.Fatal(errs)
	}
	for id, want := range map[int64]bool{heat: true, alien: true, e1: false, e2: true, e21: true} {
		if got := e.userData(id).Played == 1; got != want {
			t.Errorf("item %d played = %v, want %v", id, got, want)
		}
	}
	if d := e.userData(heat); !d.LastPlayedAt.Valid || d.LastPlayedAt.Int64 != parseTime("2026-10-20T20:00:00Z").Unix() {
		t.Errorf("heat last played: %+v", d.LastPlayedAt)
	}
	push := e.ft.find("POST", "/sync/history")
	if len(push) != 1 || !strings.Contains(push[0].body, `"tmdb":348`) || strings.Contains(push[0].body, "tt0113277") {
		t.Fatalf("history push: %+v", push)
	}
	evs, _ := e.svc.Events(e.ctx, e.user, "trakt", 10)
	kinds := map[string]bool{}
	for _, ev := range evs {
		kinds[ev.Kind] = true
	}
	if !kinds["import"] || !kinds["export"] {
		t.Errorf("events: %+v", evs)
	}
	// Incremental: remote activity unchanged → nothing fetched, local change pushed.
	if err := e.st.MarkWatched(e.ctx, db.MarkWatchedParams{UserID: e.user, ItemID: e1}); err != nil {
		t.Fatal(err)
	}
	before := e.ft.count("GET", "/sync/watched/movies")
	e.svc.LocalChanged(e.ctx, e.user)
	e.drain(JobRun)
	if e.ft.count("GET", "/sync/watched/movies") != before {
		t.Error("remote fetched although its activity did not move")
	}
	push = e.ft.find("POST", "/sync/history")
	if len(push) != 2 || !strings.Contains(push[1].body, `"episodes":[{"number":1`) {
		t.Fatalf("incremental push: %+v", push)
	}
	conn, _ := e.st.GetSyncConnectionByID(e.ctx, c.ID)
	if !conn.LastSyncAt.Valid || conn.LastError != "" {
		t.Errorf("connection after sync: %+v", conn)
	}
}

func TestConflictLatestWins(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	a := e.movie("A", `{"tmdb":"100"}`)
	b := e.movie("B", `{"tmdb":"200"}`)
	gone := e.movie("C", `{"tmdb":"300"}`)
	c := e.connect(e.trakt, Token{Access: "a1"})
	e.drain(JobRun)

	// Previous sync: A and B rated 7, C watched on both sides.
	for _, id := range []int64{a, b} {
		_ = e.st.UpsertSyncState(e.ctx, db.UpsertSyncStateParams{ConnectionID: c.ID, Feature: "ratings", ItemID: id, Value: 7})
	}
	_ = e.st.UpsertSyncState(e.ctx, db.UpsertSyncStateParams{ConnectionID: c.ID, Feature: "history", ItemID: gone})
	old := e.clock.Add(-48 * time.Hour).Unix()
	_ = e.st.SyncMarkPlayed(e.ctx, db.SyncMarkPlayedParams{UserID: e.user, ItemID: gone, At: store.NullTime(old)})
	// Local: A → 9 one hour ago, B → 6 three hours ago.
	for id, v := range map[int64]float64{a: 9, b: 6} {
		at := e.clock.Add(-time.Hour)
		if id == b {
			at = e.clock.Add(-3 * time.Hour)
		}
		_ = e.st.SyncSetRating(e.ctx, db.SyncSetRatingParams{UserID: e.user, ItemID: id, Rating: nullF(v), RatedAt: store.NullTime(at.Unix())})
	}
	// Remote: both changed two hours ago (A → 5, B → 8); C no longer watched.
	two := e.clock.Add(-2 * time.Hour).Format(time.RFC3339)
	e.ft.json("GET", "/sync/ratings/movies", 200, `[
		{"rated_at":"`+two+`","rating":5,"type":"movie","movie":{"ids":{"tmdb":100}}},
		{"rated_at":"`+two+`","rating":8,"type":"movie","movie":{"ids":{"tmdb":200}}}]`)
	if _, err := e.svc.RunNow(e.ctx, e.user, "trakt", true); err != nil {
		t.Fatal(err)
	}
	if errs := e.drain(JobRun); len(errs) > 0 {
		t.Fatal(errs)
	}
	push := e.ft.find("POST", "/sync/ratings")
	if len(push) != 1 || !strings.Contains(push[0].body, `"rating":9`) || strings.Contains(push[0].body, `"tmdb":200`) {
		t.Fatalf("ratings push: %+v", push)
	}
	if d := e.userData(b); !d.Rating.Valid || d.Rating.Float64 != 8 {
		t.Errorf("B rating = %+v (remote is newer)", d.Rating)
	}
	if d := e.userData(a); d.Rating.Float64 != 9 {
		t.Errorf("A rating = %+v (local is newer)", d.Rating)
	}
	// Removed remotely, untouched locally: removal applied.
	if e.userData(gone).Played != 0 {
		t.Error("C still played after remote removal")
	}
	evs, _ := e.svc.Events(e.ctx, e.user, "trakt", 50)
	winners := map[int64]string{}
	for _, ev := range evs {
		if ev.Kind == "conflict" && ev.ItemID != nil {
			winners[*ev.ItemID] = ev.Winner
		}
	}
	if winners[a] != "local" || winners[b] != "remote" || len(winners) != 2 {
		t.Errorf("conflicts: %+v", evs)
	}
	rows, _ := e.st.ListSyncState(e.ctx, db.ListSyncStateParams{ConnectionID: c.ID, Feature: "ratings"})
	if len(rows) != 2 {
		t.Errorf("ratings shadow: %+v", rows)
	}
}

func TestRateLimitRetryAfter(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	e.connect(e.trakt, Token{Access: "a1"})
	e.ft.on("GET", "/sync/last_activities", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	})
	errs := e.drain(JobRun)
	if len(errs) != 1 {
		t.Fatalf("errors: %v", errs)
	}
	ra, ok := errors.AsType[*jobs.RetryAfterError](errs[0])
	if !ok || ra.After != 120*time.Second || errors.Is(errs[0], jobs.ErrPermanent) {
		t.Fatalf("not a delayed retry: %v", errs[0])
	}
	// Within the window the client does not call the service at all.
	_, _ = e.svc.RunNow(e.ctx, e.user, "trakt", false)
	errs = e.drain(JobRun)
	if len(errs) != 1 || e.ft.count("GET", "/sync/last_activities") != 1 {
		t.Fatalf("called again during the pause: %d", e.ft.count("GET", "/sync/last_activities"))
	}
	v, _ := e.svc.Connection(e.ctx, e.user, "trakt")
	if !strings.Contains(v.LastError, "Limite") || v.Status != "connected" {
		t.Errorf("connection: %+v", v)
	}
	// An exhausted X-Ratelimit quota pauses too.
	e.trakt.c.pausedUntil = time.Time{}
	e.ft.on("GET", "/sync/last_activities", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Ratelimit", `{"name":"AUTHED_API_GET_LIMIT","period":300,"limit":1000,"remaining":0,"until":"`+
			e.clock.Add(90*time.Second).Format(time.RFC3339)+`"}`)
		_, _ = w.Write([]byte(`{}`))
	})
	_, _ = e.svc.RunNow(e.ctx, e.user, "trakt", false)
	errs = e.drain(JobRun)
	if len(errs) != 1 {
		t.Fatalf("quota: %v", errs)
	}
	if ra, ok := errors.AsType[*jobs.RetryAfterError](errs[0]); !ok || ra.After != 90*time.Second {
		t.Errorf("quota pause: %v", errs[0])
	}
}

func TestWatchlistAndProgress(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	m := e.movie("Dune", `{"tmdb":"438631"}`)
	m2 := e.movie("Arrival", `{"tmdb":"329865"}`)
	series := e.item("series", "Severance", `{"tvdb":"371980"}`, 0, 0, 0)
	_ = e.st.AddToWatchlist(e.ctx, db.AddToWatchlistParams{UserID: e.user, ItemID: m})
	_ = e.st.SaveProgress(e.ctx, db.SaveProgressParams{UserID: e.user, ItemID: m2, PositionMs: 3_000_000}) // 50 %
	e.ft.json("GET", "/sync/watchlist/shows", 200, `[{"listed_at":"2026-10-01T00:00:00.000Z","type":"show","show":{"ids":{"tvdb":371980}}}]`)
	e.ft.json("GET", "/sync/playback/movies", 200, `[{"id":77,"progress":25,"paused_at":"2020-01-01T00:00:00.000Z","type":"movie","movie":{"ids":{"tmdb":329865}}}]`)
	c := e.connect(e.trakt, Token{Access: "a1"})
	if errs := e.drain(JobRun); len(errs) > 0 {
		t.Fatal(errs)
	}
	ids, _ := e.st.ListWatchlistIDs(e.ctx, e.user)
	if len(ids) != 2 {
		t.Fatalf("watchlist = %v", ids)
	}
	if push := e.ft.find("POST", "/sync/watchlist"); len(push) != 1 || !strings.Contains(push[0].body, `"tmdb":438631`) {
		t.Fatalf("watchlist push: %+v", push)
	}
	_ = series
	// Local progress (now) is newer than Trakt's (2020): pushed as a pause.
	if p := e.ft.find("POST", "/scrobble/pause"); len(p) != 1 || !strings.Contains(p[0].body, `"progress":50`) {
		t.Fatalf("progress push: %+v", p)
	}
	// Removing from the local watchlist removes it remotely.
	_ = e.st.RemoveFromWatchlist(e.ctx, db.RemoveFromWatchlistParams{UserID: e.user, ItemID: m})
	e.ft.json("GET", "/sync/watchlist/movies", 200, `[{"listed_at":"2026-10-28T00:00:00.000Z","type":"movie","movie":{"ids":{"tmdb":438631}}}]`)
	_, _ = e.svc.RunNow(e.ctx, e.user, "trakt", true)
	if errs := e.drain(JobRun); len(errs) > 0 {
		t.Fatal(errs)
	}
	if rm := e.ft.find("POST", "/sync/watchlist/remove"); len(rm) != 1 || !strings.Contains(rm[0].body, `"tmdb":438631`) {
		t.Fatalf("watchlist remove: %+v (state %+v)", rm, c)
	}
}

func TestSimklPinAndHistory(t *testing.T) {
	e := newEnv(t)
	simklEmpty(e.fs)
	e.fs.json("GET", "/oauth/pin", 200, `{"result":"OK","device_code":"DEVICE_CODE","user_code":"7K3Q9","verification_uri":"https://simkl.com/pin",
		"verification_url":"https://simkl.com/pin","expires_in":900,"interval":5}`)
	polls := 0
	e.fs.on("GET", "/oauth/pin/7K3Q9", func(w http.ResponseWriter, _ *http.Request) {
		polls++
		if polls == 1 {
			_, _ = w.Write([]byte(`{"result":"KO","message":"Authorization pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":"OK","access_token":"simkl-token"}`))
	})
	movie := e.movie("Heat", `{"imdb":"tt0113277"}`)
	old := e.movie("Alien", `{"imdb":"tt0078748"}`)
	series := e.item("series", "Breaking Bad", `{"tvdb":"81189"}`, 0, 0, 0)
	ep := e.episode(series, 5, 16)
	anime := e.item("series", "Attack on Titan", `{"tvdb":"267440"}`, 0, 0, 0)
	aep := e.episode(anime, 2, 4)
	e.fs.json("GET", "/sync/all-items/movies", 200, `{"movies":[{"status":"completed","last_watched_at":"2026-10-20T00:00:00Z",
		"user_rating":8,"user_rated_at":"2026-10-20T00:00:00Z","movie":{"title":"Heat","ids":{"simkl":5,"imdb":"tt0113277","tmdb":"949"}}},
		{"status":"completed","last_watched_at":"1970-01-01T00:00:01Z","user_rating":null,"movie":{"title":"Alien","ids":{"simkl":6,"imdb":"tt0078748"}}}]}`)
	// A completed show only carries its episodes with include_all_episodes.
	e.fs.on("GET", "/sync/all-items/shows", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("include_all_episodes") != "yes" {
			_, _ = w.Write([]byte(`{"shows":[{"status":"completed","show":{"ids":{"simkl":11,"tvdb":"81189"}}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"shows":[{"status":"completed","last_watched_at":"2026-10-21T00:00:00Z","show":{"ids":{"simkl":11,"tvdb":"81189"}},
			"seasons":[{"number":5,"episodes":[{"number":16,"watched_at":"2026-10-21T00:00:00Z"}]}]}]}`))
	})
	// Anime: per-cour numbering (S2 is its own title, episode 4 of season 1),
	// mapped to TVDB numbering with extended=full_anime_seasons.
	e.fs.json("GET", "/sync/all-items/anime", 200, `{"anime":[{"status":"watching","show":{"ids":{"simkl":39687,"tvdb":"267440","mal":"20958"}},
		"anime_type":"tv","mapped_tvdb_seasons":[2],"seasons":[{"number":1,"episodes":[{"number":4,"watched_at":"2026-10-22T00:00:00Z",
		"tvdb":{"season":2,"episode":4}}]}]}]}`)

	pend, err := e.svc.Connect(e.ctx, e.user, "simkl")
	if err != nil {
		t.Fatal(err)
	}
	if pend.UserCode != "7K3Q9" || pend.VerificationURL != "https://simkl.com/pin" {
		t.Fatalf("pending: %+v", pend)
	}
	if res, _ := e.svc.Poll(e.ctx, e.user, "simkl"); res.Status != "pending" {
		t.Fatalf("poll 1: %+v", res)
	}
	e.clock = e.clock.Add(6 * time.Second)
	res, err := e.svc.Poll(e.ctx, e.user, "simkl")
	if err != nil || res.Status != "connected" || res.Connection.Account.Username != "alice_s" || res.Connection.Features[FeatProgress] {
		t.Fatalf("poll 2: %+v %v", res, err)
	}
	if errs := e.drain(JobRun); len(errs) > 0 {
		t.Fatal(errs)
	}
	d := e.userData(movie)
	if d.Played != 1 || !d.Rating.Valid || d.Rating.Float64 != 8 {
		t.Errorf("imported: %+v", d)
	}
	if d := e.userData(old); d.Played != 1 {
		t.Errorf("watched with unknown date: %+v", d)
	}
	if d := e.userData(ep); d.Played != 1 {
		t.Errorf("episode of a completed show: %+v", d)
	}
	if d := e.userData(aep); d.Played != 1 {
		t.Errorf("anime episode (TVDB S02E04): %+v", d)
	}
	if got := e.fs.find("GET", "/sync/all-items/movies"); len(got) != 1 || got[0].auth != "Bearer simkl-token" {
		t.Errorf("auth: %+v", got)
	}
	if got := e.fs.find("GET", "/sync/all-items/anime"); len(got) != 1 || !strings.Contains(got[0].query, "extended=full_anime_seasons") {
		t.Errorf("anime query: %+v", got)
	}
	// Every call identifies the application in its query string.
	e.fs.mu.Lock()
	for _, c := range e.fs.calls {
		if !strings.Contains(c.query, "client_id=cid-simkl") || !strings.Contains(c.query, "app-name=aether") || !strings.Contains(c.query, "app-version=") {
			t.Errorf("%s %s: missing app parameters (%s)", c.method, c.path, c.query)
		}
	}
	e.fs.mu.Unlock()

	// Live scrobbling with Simkl's own endpoints.
	e.svc.Scrobble(e.ctx, e.user, movie, ActStart, 12.345)
	e.svc.Scrobble(e.ctx, e.user, ep, ActStop, 95)
	if errs := e.drain(JobScrobble); len(errs) > 0 {
		t.Fatal(errs)
	}
	if st := e.fs.find("POST", "/scrobble/start"); len(st) != 1 || !strings.Contains(st[0].body, `"imdb":"tt0113277"`) ||
		!strings.Contains(st[0].body, `"progress":12.35`) {
		t.Errorf("scrobble start: %+v", st)
	}
	if st := e.fs.find("POST", "/scrobble/stop"); len(st) != 1 || !strings.Contains(st[0].body, `"tvdb":81189`) ||
		!strings.Contains(st[0].body, `"episode":{"number":16,"season":5}`) {
		t.Errorf("scrobble stop: %+v", st)
	}
	if n := e.fs.count("POST", "/sync/history"); n != 0 {
		t.Errorf("scrobble also pushed to history (%d)", n)
	}
	// A second stop within the hour (409) is not an error.
	e.fs.json("POST", "/scrobble/stop", 409, `{"error":"already_watched","watched_at":"2026-10-28T10:00:00Z","expires_at":"2026-10-28T11:00:00Z"}`)
	e.svc.Scrobble(e.ctx, e.user, ep, ActStop, 96)
	if errs := e.drain(JobScrobble); len(errs) > 0 {
		t.Errorf("409: %v", errs)
	}

	// Disconnect: no revocation endpoint, the link is simply forgotten.
	if err := e.svc.Disconnect(e.ctx, e.user, "simkl"); err != nil {
		t.Fatal(err)
	}
	if v, _ := e.svc.Connection(e.ctx, e.user, "simkl"); v.Status != "disconnected" {
		t.Errorf("after disconnect: %+v", v)
	}
}

func TestSimklPinGone(t *testing.T) {
	e := newEnv(t)
	e.fs.json("GET", "/oauth/pin", 200, `{"result":"OK","device_code":"DEVICE_CODE","user_code":"AAAAA","verification_uri":"https://simkl.com/pin","expires_in":900,"interval":5}`)
	// An unknown or collected code is answered with a fresh code.
	e.fs.json("GET", "/oauth/pin/AAAAA", 200, `{"result":"OK","device_code":"DEVICE_CODE","user_code":"BBBBB","verification_uri":"https://simkl.com/pin","expires_in":900,"interval":5}`)
	if _, err := e.svc.Connect(e.ctx, e.user, "simkl"); err != nil {
		t.Fatal(err)
	}
	if res, _ := e.svc.Poll(e.ctx, e.user, "simkl"); res.Status != "expired" {
		t.Fatalf("gone code: %+v", res)
	}
	// An AUTH V2 client_id is refused by the PIN flow with a clear message.
	e.fs.json("GET", "/oauth/pin", 400, `{"error":"unauthorized_client","code":400,"message":"This client_id is an OAuth 2.0 app: use POST /oauth2/device instead of /oauth/pin"}`)
	if _, err := e.svc.Connect(e.ctx, e.user, "simkl"); err == nil || !strings.Contains(err.Error(), "AUTH V2") {
		t.Errorf("V2 client id: %v", err)
	}
}

func TestTraktPagination(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	m1 := e.movie("Heat", `{"tmdb":"949"}`)
	m2 := e.movie("Alien", `{"tmdb":"348"}`)
	e.ft.on("GET", "/sync/ratings/movies", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("limit") == "" {
			// Without pagination parameters Trakt may return a small first page.
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.Header().Set("X-Pagination-Page", q.Get("page"))
		w.Header().Set("X-Pagination-Page-Count", "2")
		w.Header().Set("X-Pagination-Limit", "1")
		w.Header().Set("X-Pagination-Item-Count", "2")
		if q.Get("page") == "1" {
			_, _ = w.Write([]byte(`[{"rated_at":"2026-10-01T00:00:00.000Z","rating":7,"type":"movie","movie":{"ids":{"tmdb":949}}}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"rated_at":"2026-10-02T00:00:00.000Z","rating":9,"type":"movie","movie":{"ids":{"tmdb":348}}}]`))
	})
	e.connect(e.trakt, Token{Access: "a1"})
	if errs := e.drain(JobRun); len(errs) > 0 {
		t.Fatal(errs)
	}
	if d := e.userData(m1); !d.Rating.Valid || d.Rating.Float64 != 7 {
		t.Errorf("page 1: %+v", d)
	}
	if d := e.userData(m2); !d.Rating.Valid || d.Rating.Float64 != 9 {
		t.Errorf("page 2: %+v", d)
	}
	if n := e.ft.count("GET", "/sync/ratings/movies"); n != 2 {
		t.Errorf("ratings pages fetched: %d", n)
	}
}

func TestDisconnectRevokesTrakt(t *testing.T) {
	e := newEnv(t)
	traktEmpty(e.ft)
	e.ft.json("POST", "/oauth/revoke", 200, `{}`)
	e.connect(e.trakt, Token{Access: "a1", Refresh: "r1"})
	if err := e.svc.Disconnect(e.ctx, e.user, "trakt"); err != nil {
		t.Fatal(err)
	}
	rv := e.ft.find("POST", "/oauth/revoke")
	if len(rv) != 1 || !strings.Contains(rv[0].body, `"token":"a1"`) {
		t.Fatalf("revoke: %+v", rv)
	}
	if err := e.svc.Disconnect(e.ctx, e.user, "trakt"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("second disconnect: %v", err)
	}
}

func TestAppCredentials(t *testing.T) {
	e := newEnv(t)
	if err := e.svc.SetApp(e.ctx, "trakt", Creds{ClientID: "x", ClientSecret: "y"}); !errors.Is(err, ErrManagedByConfig) {
		t.Errorf("config wins: %v", err)
	}
	e.svc.opts.Config = nil
	if e.svc.Available(e.ctx, "trakt") {
		t.Error("available without credentials")
	}
	if err := e.svc.SetApp(e.ctx, "trakt", Creds{ClientID: "abcdefghijkl"}); !errors.Is(err, ErrInvalidCreds) {
		t.Errorf("trakt without secret: %v", err)
	}
	if err := e.svc.SetApp(e.ctx, "trakt", Creds{ClientID: "abcdefghijkl", ClientSecret: "s3cr3t-value"}); err != nil {
		t.Fatal(err)
	}
	apps, _ := e.svc.Apps(e.ctx)
	if !apps["trakt"].Configured || apps["trakt"].Source != "database" || apps["trakt"].ClientID != "abcd…ijkl" || apps["simkl"].Configured {
		t.Errorf("apps: %+v", apps)
	}
	row, _ := e.st.GetSyncApp(e.ctx, "trakt")
	if bytes.Contains(row.Secret, []byte("s3cr3t-value")) {
		t.Error("client secret stored in clear")
	}
	if _, err := e.svc.Connect(e.ctx, e.user, "unknown"); !errors.Is(err, ErrUnknownService) {
		t.Errorf("unknown service: %v", err)
	}
}

func TestIndexMatching(t *testing.T) {
	ix := newIndex([]store.SyncItem{
		{ID: 1, Kind: "movie", ProviderIDs: `{"tmdb":"949","imdb":"tt0113277"}`},
		{ID: 2, Kind: "movie", ProviderIDs: `{"imdb":"tt0078748"}`},
		{ID: 3, Kind: "series", ProviderIDs: `{"tvdb":"81189","tmdb":"1396"}`},
		{ID: 4, Kind: "episode", SeriesID: 3, Season: 1, Episode: 1},
		{ID: 5, Kind: "episode", SeriesID: 3, Season: 1, Episode: 2, EpisodeEnd: 3},
		{ID: 6, Kind: "episode", SeriesID: 99, Season: 1, Episode: 1}, // series not visible
	})
	cases := []struct {
		ref  Ref
		want int64
	}{
		{Ref{Kind: "movie", IDs: IDs{Tmdb: "949"}}, 1},
		{Ref{Kind: "movie", IDs: IDs{Tmdb: "1", Imdb: "tt0078748"}}, 2}, // falls back to imdb
		{Ref{Kind: "movie", IDs: IDs{Trakt: "5"}}, 0},
		{Ref{Kind: "show", IDs: IDs{Tmdb: "1396"}}, 3},
		{Ref{Kind: "movie", IDs: IDs{Tvdb: "81189"}}, 0}, // a show id is not a movie
		{Ref{Kind: "episode", IDs: IDs{Tvdb: "81189"}, Season: 1, Episode: 1}, 4},
		{Ref{Kind: "episode", IDs: IDs{Tvdb: "81189"}, Season: 1, Episode: 3}, 5},
		{Ref{Kind: "episode", IDs: IDs{Tvdb: "81189"}, Season: 2, Episode: 1}, 0},
	}
	for _, c := range cases {
		if got, _ := ix.match(c.ref); got != c.want {
			t.Errorf("match(%+v) = %d, want %d", c.ref, got, c.want)
		}
	}
	if r := ix.refs(5); len(r) != 2 || r[1].Episode != 3 || r[0].IDs.Tvdb != "81189" {
		t.Errorf("refs(5) = %+v", r)
	}
	if r := ix.refs(6); r != nil {
		t.Errorf("refs(6) = %+v", r)
	}
}

func TestMergeRules(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	local := map[int64]state{
		1: {present: true, at: at(1)},  // new locally → push
		2: {present: true, at: at(1)},  // removed remotely, untouched locally → remove locally
		4: {present: false, at: at(5)}, // removed locally, unchanged remotely → push removal
	}
	remote := map[int64]state{
		3: {present: true, at: at(2)}, // new remotely → pull
		4: {present: true, at: at(3), ref: "x"},
	}
	shadow := map[int64]state{2: {present: true}, 4: {present: true}}
	p := merge(local, remote, shadow, at(4), nil)
	if _, ok := p.pushAdd[1]; !ok || len(p.pushAdd) != 1 {
		t.Errorf("pushAdd = %+v", p.pushAdd)
	}
	if len(p.localRemove) != 1 || p.localRemove[0] != 2 {
		t.Errorf("localRemove = %+v", p.localRemove)
	}
	if _, ok := p.localSet[3]; !ok {
		t.Errorf("localSet = %+v", p.localSet)
	}
	if r, ok := p.pushRemove[4]; !ok || r.ref != "x" {
		t.Errorf("pushRemove = %+v", p.pushRemove)
	}
	if _, ok := p.shadow[4]; ok || len(p.shadow) != 2 {
		t.Errorf("shadow = %+v", p.shadow)
	}
}

func nullF(v float64) sql.NullFloat64 { return sql.NullFloat64{Float64: v, Valid: true} }
