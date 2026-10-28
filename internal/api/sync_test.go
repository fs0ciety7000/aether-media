package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/syncsvc"
)

// syncServer is an API server whose Trakt is a local fake (no network).
func syncServer(t *testing.T) *httptest.Server {
	t.Helper()
	fake := http.NewServeMux()
	fake.HandleFunc("POST /oauth/device/code", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"device_code":"dev-code-secret","user_code":"AB12CD34","verification_url":"https://trakt.tv/activate","expires_in":600,"interval":5}`)
	})
	fake.HandleFunc("POST /oauth/device/token", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"access_token":"access-secret","refresh_token":"refresh-secret","expires_in":86400}`)
	})
	fake.HandleFunc("GET /users/settings", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"user":{"username":"bob_t","ids":{"slug":"bob-t"}}}`)
	})
	fake.HandleFunc("POST /oauth/revoke", func(http.ResponseWriter, *http.Request) {})
	fts := httptest.NewServer(fake)
	t.Cleanup(fts.Close)

	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secret.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	svc := syncsvc.New(st, box, jobs.NewRunner(st, log), events.NewHub(), log, syncsvc.Options{
		Config: map[string]syncsvc.Creds{"trakt": {ClientID: "trakt-client-id", ClientSecret: "trakt-client-secret"}},
	}, syncsvc.NewTrakt(fts.URL, "test", nil), syncsvc.NewSimkl(fts.URL, "test", "test", nil))
	ts := httptest.NewServer(New(Deps{Store: st, Log: log, Hub: events.NewHub(), Sync: svc}).Handler(http.NotFoundHandler()))
	t.Cleanup(ts.Close)
	authed = loginAdmin(t, ts)
	return ts
}

func TestSyncAPI(t *testing.T) {
	ts := syncServer(t)
	admin := authed
	bob := member(t, ts, admin, `{"name":"bob","password":"motdepasse"}`)
	secrets := []string{"access-secret", "refresh-secret", "dev-code-secret", "trakt-client-secret", "simkl-client-id-0001"}
	noSecret := func(where string, b []byte) {
		t.Helper()
		for _, s := range secrets {
			if strings.Contains(string(b), s) {
				t.Errorf("%s leaks %s: %s", where, s, b)
			}
		}
	}

	// Application credentials: admin only; configuration wins.
	if code, _ := send(t, bob, "GET", ts.URL+"/api/v1/sync/apps", ""); code != 403 {
		t.Errorf("member apps → %d", code)
	}
	if code, _ := send(t, bob, "PUT", ts.URL+"/api/v1/sync/apps/simkl", `{"clientId":"x"}`); code != 403 {
		t.Errorf("member put app → %d", code)
	}
	if code, b := send(t, admin, "PUT", ts.URL+"/api/v1/sync/apps/trakt", `{"clientId":"abcdefghij","clientSecret":"z"}`); code != 409 || !strings.Contains(string(b), "managed_by_config") {
		t.Errorf("override config → %d %s", code, b)
	}
	code, b := send(t, admin, "PUT", ts.URL+"/api/v1/sync/apps/simkl", `{"clientId":"simkl-client-id-0001"}`)
	if code != 200 || !strings.Contains(string(b), `"source":"database"`) {
		t.Fatalf("put simkl → %d %s", code, b)
	}
	noSecret("put app", b)
	_, b = send(t, admin, "GET", ts.URL+"/api/v1/sync/apps", "")
	noSecret("apps", b)
	_, b = send(t, http.DefaultClient, "GET", ts.URL+"/api/v1/system/info", "")
	if !strings.Contains(string(b), `"sync":{"trakt":true,"simkl":true}`) {
		t.Errorf("system info: %s", b)
	}

	// A user's connections.
	var conns []syncsvc.Connection
	_, b = send(t, bob, "GET", ts.URL+"/api/v1/me/sync", "")
	if err := json.Unmarshal(b, &conns); err != nil || len(conns) != 2 || conns[0].Service != "trakt" || conns[0].Status != "disconnected" || !conns[0].Available {
		t.Fatalf("me/sync: %s", b)
	}
	code, b = send(t, bob, "POST", ts.URL+"/api/v1/me/sync/trakt/connect", "")
	if code != 200 || !strings.Contains(string(b), `"userCode":"AB12CD34"`) {
		t.Fatalf("connect → %d %s", code, b)
	}
	noSecret("connect", b)
	code, b = send(t, bob, "POST", ts.URL+"/api/v1/me/sync/trakt/poll", "")
	if code != 200 || !strings.Contains(string(b), `"status":"connected"`) || !strings.Contains(string(b), `"username":"bob_t"`) {
		t.Fatalf("poll → %d %s", code, b)
	}
	noSecret("poll", b)
	_, b = send(t, bob, "GET", ts.URL+"/api/v1/me/sync", "")
	noSecret("me/sync", b)
	if code, _ := send(t, bob, "POST", ts.URL+"/api/v1/me/sync/trakt/poll", ""); code != 404 {
		t.Errorf("poll without pairing → %d", code)
	}

	// Isolation: the admin sees their own (absent) link only.
	_, b = send(t, admin, "GET", ts.URL+"/api/v1/me/sync", "")
	if strings.Contains(string(b), "bob_t") || !strings.Contains(string(b), `"status":"disconnected"`) {
		t.Errorf("admin sees bob's link: %s", b)
	}
	if code, _ := send(t, admin, "POST", ts.URL+"/api/v1/me/sync/trakt/run", ""); code != 404 {
		t.Errorf("admin run without link → %d", code)
	}

	// Options, run, journal.
	code, b = send(t, bob, "PATCH", ts.URL+"/api/v1/me/sync/trakt", `{"features":{"progress":false}}`)
	if code != 200 || !strings.Contains(string(b), `"progress":false`) || !strings.Contains(string(b), `"history":true`) {
		t.Errorf("patch → %d %s", code, b)
	}
	if code, _ := send(t, bob, "PATCH", ts.URL+"/api/v1/me/sync/simkl", `{"features":{"progress":true}}`); code != 404 {
		t.Errorf("patch unconnected → %d", code)
	}
	if code, b := send(t, bob, "PATCH", ts.URL+"/api/v1/me/sync/trakt", `{"features":{"teleport":true}}`); code != 400 || !strings.Contains(string(b), "unsupported_feature") {
		t.Errorf("patch unknown feature → %d %s", code, b)
	}
	if code, b := send(t, bob, "POST", ts.URL+"/api/v1/me/sync/trakt/run", `{"full":true}`); code != 202 || !strings.Contains(string(b), "queued") {
		t.Errorf("run → %d %s", code, b)
	}
	if code, b := send(t, bob, "GET", ts.URL+"/api/v1/me/sync/trakt/events", ""); code != 200 || string(b) != "[]\n" {
		t.Errorf("events → %d %s", code, b)
	}
	if code, b := send(t, bob, "POST", ts.URL+"/api/v1/me/sync/myspace/connect", ""); code != 404 || !strings.Contains(string(b), "unknown_service") {
		t.Errorf("unknown service → %d %s", code, b)
	}
	if code, _ := send(t, bob, "DELETE", ts.URL+"/api/v1/me/sync/trakt", ""); code != 204 {
		t.Errorf("disconnect → %d", code)
	}
	if code, _ := send(t, bob, "DELETE", ts.URL+"/api/v1/me/sync/trakt", ""); code != 404 {
		t.Errorf("second disconnect → %d", code)
	}
	if code, _ := send(t, http.DefaultClient, "GET", ts.URL+"/api/v1/me/sync", ""); code != 401 {
		t.Errorf("anonymous → %d", code)
	}
}

func TestWatchlistAPI(t *testing.T) {
	ts := browseServer(t)
	var page itemPage
	getJSON(t, ts, "/api/v1/items", &page)
	id := itoa(page.Items[0].ID)
	if code, b := send(t, authed, "PUT", ts.URL+"/api/v1/watchlist/"+id, ""); code != 204 {
		t.Fatalf("add → %d %s", code, b)
	}
	send(t, authed, "PUT", ts.URL+"/api/v1/watchlist/"+id, "") // idempotent
	_, b := send(t, authed, "GET", ts.URL+"/api/v1/watchlist", "")
	var list []struct {
		ID       int64
		UserData struct{ InWatchlist bool }
	}
	if err := json.Unmarshal(b, &list); err != nil || len(list) != 1 || !list[0].UserData.InWatchlist {
		t.Fatalf("watchlist: %s", b)
	}
	getJSON(t, ts, "/api/v1/items?watchlist=true", &page)
	if len(page.Items) != 1 {
		t.Errorf("items?watchlist=true → %d", len(page.Items))
	}
	// Per user.
	bob := member(t, ts, authed, `{"name":"bob","password":"motdepasse"}`)
	if _, b := send(t, bob, "GET", ts.URL+"/api/v1/watchlist", ""); string(b) != "[]\n" {
		t.Errorf("bob's watchlist: %s", b)
	}
	if code, _ := send(t, authed, "DELETE", ts.URL+"/api/v1/watchlist/"+id, ""); code != 204 {
		t.Errorf("remove → %d", code)
	}
	if _, b := send(t, authed, "GET", ts.URL+"/api/v1/watchlist", ""); string(b) != "[]\n" {
		t.Errorf("after remove: %s", b)
	}
	if code, _ := send(t, authed, "PUT", ts.URL+"/api/v1/watchlist/999999", ""); code != 404 {
		t.Errorf("unknown item → %d", code)
	}
	// Ratings can be cleared with null.
	send(t, authed, "PUT", ts.URL+"/api/v1/items/"+id+"/userdata", `{"rating":7}`)
	_, b = send(t, authed, "PUT", ts.URL+"/api/v1/items/"+id+"/userdata", `{"rating":null}`)
	if strings.Contains(string(b), `"rating"`) {
		t.Errorf("rating not cleared: %s", b)
	}
	if _, b = send(t, authed, "PUT", ts.URL+"/api/v1/items/"+id+"/userdata", `{"favorite":true}`); strings.Contains(string(b), `"rating"`) {
		t.Errorf("absent rating changed it: %s", b)
	}
}
