package syncsvc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/metadata/httpx"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// ---- fake remote API (no real network in tests) ----

type call struct {
	method, path, query, body, auth string
}

type fakeAPI struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	calls  []call
	srv    *httptest.Server
}

func newFake(t *testing.T) *fakeAPI {
	f := &fakeAPI{t: t, routes: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		f.mu.Lock()
		f.calls = append(f.calls, call{r.Method, r.URL.Path, r.URL.RawQuery, string(b), r.Header.Get("Authorization")})
		h := f.routes[key]
		f.mu.Unlock()
		if h == nil {
			t.Logf("fake: route inconnue %s", key)
			http.NotFound(w, r)
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(b)))
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) on(method, path string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = h
}

func (f *fakeAPI) json(method, path string, status int, body string) {
	f.on(method, path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	})
}

func (f *fakeAPI) find(method, path string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []call
	for _, c := range f.calls {
		if c.method == method && c.path == path {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeAPI) count(method, path string) int { return len(f.find(method, path)) }

// traktEmpty registers empty lists for every sync endpoint.
func traktEmpty(f *fakeAPI) {
	f.json("GET", "/sync/last_activities", 200, `{"all":"2026-10-01T00:00:00.000Z","movies":{"watched_at":"2026-10-01T00:00:00.000Z"}}`)
	for _, p := range []string{"/sync/watched/movies", "/sync/watched/shows", "/sync/playback/movies", "/sync/playback/episodes",
		"/sync/watchlist/movies", "/sync/watchlist/shows", "/sync/ratings/movies", "/sync/ratings/shows", "/sync/ratings/episodes"} {
		f.json("GET", p, 200, `[]`)
	}
	for _, p := range []string{"/sync/history", "/sync/history/remove", "/sync/watchlist", "/sync/watchlist/remove",
		"/sync/ratings", "/sync/ratings/remove", "/scrobble/start", "/scrobble/pause", "/scrobble/stop"} {
		f.json("POST", p, 201, `{}`)
	}
	f.json("GET", "/users/settings", 200, `{"user":{"username":"alice_t","ids":{"slug":"alice-t"}}}`)
}

// ---- test environment ----

type env struct {
	t      *testing.T
	ctx    context.Context
	st     *store.Store
	box    *secret.Box
	svc    *Service
	trakt  *Trakt
	simkl  *Simkl
	ft, fs *fakeAPI
	clock  time.Time
	user   int64
	lib    int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secret.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, ctx: ctx, st: st, box: box, ft: newFake(t), fs: newFake(t), clock: time.Now().UTC().Truncate(time.Second)}
	e.trakt = NewTrakt(e.ft.srv.URL, "Aether-test", nil)
	e.trakt.c.writes, e.trakt.c.reads = httpx.NewLimiter(1000), httpx.NewLimiter(1000)
	e.trakt.c.now = func() time.Time { return e.clock }
	e.simkl = NewSimkl(e.fs.srv.URL, "Aether-test", "test", nil)
	e.simkl.c.writes, e.simkl.c.reads = httpx.NewLimiter(1000), httpx.NewLimiter(1000)
	log := slog.New(slog.DiscardHandler)
	e.svc = New(st, box, jobs.NewRunner(st, log), nil, log, Options{
		Config:     map[string]Creds{"trakt": {ClientID: "cid-trakt", ClientSecret: "csecret-trakt"}, "simkl": {ClientID: "cid-simkl"}},
		LocalDelay: -1,
	}, e.trakt, e.simkl)
	e.svc.now = func() time.Time { return e.clock }
	e.user = e.exec(`INSERT INTO users (name, password_hash) VALUES ('alice', 'x')`)
	e.lib = e.exec(`INSERT INTO libraries (name, kind) VALUES ('Tout', 'mixed')`)
	return e
}

func (e *env) exec(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.st.DB.ExecContext(e.ctx, q, args...)
	if err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (e *env) item(kind, title, ids string, parent int64, index int64, runtime int64) int64 {
	var p any
	if parent > 0 {
		p = parent
	}
	return e.exec(`INSERT INTO items (library_id, parent_id, kind, title, sort_title, provider_ids, index_number, runtime_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, e.lib, p, kind, title, title, ids, index, runtime)
}

func (e *env) movie(title, ids string) int64 { return e.item("movie", title, ids, 0, 0, 6_000_000) }

// episode creates series → season → episode and returns the episode id.
func (e *env) episode(series int64, season, number int64) int64 {
	var sid int64
	err := e.st.DB.QueryRowContext(e.ctx, `SELECT id FROM items WHERE parent_id = ? AND kind = 'season' AND index_number = ?`, series, season).Scan(&sid)
	if errors.Is(err, sql.ErrNoRows) {
		sid = e.item("season", fmt.Sprintf("Saison %d", season), "{}", series, season, 0)
	}
	return e.item("episode", fmt.Sprintf("S%02dE%02d", season, number), "{}", sid, number, 2_400_000)
}

// connect links the user to a provider as a finished pairing would, then
// runs the initial synchronisation.
func (e *env) connect(p Provider, tok Token) db.SyncConnection {
	e.t.Helper()
	c, err := e.st.UpsertSyncPending(e.ctx, db.UpsertSyncPendingParams{UserID: e.user, Service: p.Service(),
		PendingSecret: []byte("x"), PendingExpiresAt: store.NullTime(e.clock.Add(time.Hour).Unix()), PendingInterval: 5})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.svc.saveConnected(e.ctx, p, &c, tok, Account{Username: "alice_remote", ID: "alice-remote"}); err != nil {
		e.t.Fatal(err)
	}
	c, err = e.st.GetSyncConnectionByID(e.ctx, c.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

// drain runs queued jobs of a kind (ignoring their delay) and returns their errors.
func (e *env) drain(kind string) []error {
	e.t.Helper()
	if _, err := e.st.DB.ExecContext(e.ctx, `UPDATE jobs SET run_after = 0 WHERE kind = ? AND status = 'queued'`, kind); err != nil {
		e.t.Fatal(err)
	}
	h := map[string]jobs.Handler{JobRun: e.svc.handleRun, JobScrobble: e.svc.handleScrobble}[kind]
	var errs []error
	for range 50 {
		row, err := e.st.ClaimJob(e.ctx, kind)
		if errors.Is(err, sql.ErrNoRows) {
			return errs
		}
		if err != nil {
			e.t.Fatal(err)
		}
		if err := h(e.ctx, jobs.Job{ID: row.ID, Kind: row.Kind, Key: row.Key, Payload: []byte(row.Payload), Attempts: int(row.Attempts)}); err != nil {
			errs = append(errs, err)
		}
		if err := e.st.DeleteJob(e.ctx, row.ID); err != nil {
			e.t.Fatal(err)
		}
	}
	e.t.Fatal("drain: trop de tâches")
	return nil
}

func (e *env) userData(item int64) db.UserItemDatum {
	e.t.Helper()
	d, err := e.st.GetUserData(e.ctx, db.GetUserDataParams{UserID: e.user, ItemID: item})
	if errors.Is(err, sql.ErrNoRows) {
		return db.UserItemDatum{}
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}
