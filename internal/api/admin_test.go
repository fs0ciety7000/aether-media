package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/config"
	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/logbuf"
	"github.com/fs0ciety7000/aether-media/internal/metadata/tmdb"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/settings"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store"
)

const goodKey = "0123456789abcdef0123456789abcdef"

type adminEnv struct {
	ts     *httptest.Server
	runner *jobs.Runner
	logs   *logbuf.Buffer
	log    *slog.Logger
	store  *store.Store
}

func adminServer(t *testing.T) adminEnv {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("AETHER_DATA_DIR", dir)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	logs := logbuf.New(100)
	log := slog.New(logs.Handler(slog.LevelDebug))
	hub := events.NewHub()
	srcs, err := sources.NewForTests(st, dir, hub, log)
	if err != nil {
		t.Fatal(err)
	}
	box, err := secret.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	set, err := settings.New(ctx, st, box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Fake TMDB: only goodKey is accepted.
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/authentication" && r.URL.Query().Get("api_key") == goodKey {
			io.WriteString(w, `{"success":true}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"status_code":7,"status_message":"Invalid API key"}`)
	}))
	t.Cleanup(fake.Close)
	tm := tmdb.New(tmdb.Config{BaseURL: fake.URL}, nil)
	set.SetHooks(settings.Hooks{TMDBKey: tm.SetAPIKey, CheckTMDB: tm.CheckKey})

	runner := jobs.NewRunner(st, log)
	runner.Register("test.fail", 1, func(context.Context, jobs.Job) error {
		return fmt.Errorf("%w: boom api_key=%s", jobs.ErrPermanent, goodKey)
	})
	runner.Register("test.block", 1, func(ctx context.Context, _ jobs.Job) error {
		<-ctx.Done()
		return ctx.Err()
	})
	web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "spa") })
	ts := httptest.NewServer(New(Deps{Store: st, Log: log, Sources: srcs, Hub: hub, Settings: set, Runner: runner, Logs: logs}).Handler(web))
	t.Cleanup(ts.Close)
	authed = loginAdmin(t, ts)
	return adminEnv{ts: ts, runner: runner, logs: logs, log: log, store: st}
}

func TestAdminRoutesAreAdminOnly(t *testing.T) {
	env := adminServer(t)
	m := member(t, env.ts, authed, `{"name":"lea","password":"motdepasse"}`)
	for _, rt := range []string{"GET /api/v1/settings", "PATCH /api/v1/settings", "POST /api/v1/settings/tmdb/check", "GET /api/v1/system/status",
		"GET /api/v1/jobs/queue", "POST /api/v1/jobs/run", "POST /api/v1/jobs/1/retry", "DELETE /api/v1/jobs/1",
		"GET /api/v1/logs", "GET /api/v1/logs/stream", "GET /api/v1/logs/download", "GET /api/v1/playback/sessions", "POST /api/v1/playback/sessions/x/stop"} {
		method, path, _ := strings.Cut(rt, " ")
		if code, _ := send(t, m, method, env.ts.URL+path, `{}`); code != http.StatusForbidden {
			t.Errorf("%s as member: %d", rt, code)
		}
	}
}

func TestSettingsAPI(t *testing.T) {
	env := adminServer(t)
	u := env.ts.URL + "/api/v1"
	// The assistant resumes after the admin account (created by loginAdmin).
	code, b := send(t, authed, "GET", u+"/system/info", "")
	if code != 200 || !strings.Contains(string(b), `"setupWizardPending":true`) || !strings.Contains(string(b), `"tmdb":false`) {
		t.Fatalf("info: %d %s", code, b)
	}
	code, b = send(t, authed, "POST", u+"/settings/tmdb/check", `{"apiKey":"nope-nope-nope"}`)
	if code != 200 || !strings.Contains(string(b), `"error":"invalid_key"`) {
		t.Fatalf("check bad: %d %s", code, b)
	}
	code, b = send(t, authed, "POST", u+"/settings/tmdb/check", `{"apiKey":"`+goodKey+`"}`)
	if code != 200 || !strings.Contains(string(b), `"valid":true`) {
		t.Fatalf("check good: %d %s", code, b)
	}
	code, b = send(t, authed, "PATCH", u+"/settings", `{"tmdbApiKey":"`+goodKey+`","metadataLanguage":"fr-CA","metadataCountry":"CA","wizard":{"step":"library"}}`)
	if code != 200 || strings.Contains(string(b), goodKey) {
		t.Fatalf("patch: %d %s", code, b)
	}
	var v settings.View
	_ = json.Unmarshal(b, &v)
	if !v.TMDBAPIKey.Set || v.TMDBAPIKey.Masked != "…cdef" || v.MetadataLanguage.Value != "fr-CA" || v.Wizard.Step != "library" {
		t.Fatalf("view: %+v", v)
	}
	code, b = send(t, authed, "POST", u+"/settings/tmdb/check", ``) // the stored key
	if code != 200 || !strings.Contains(string(b), `"valid":true`) {
		t.Fatalf("check stored: %d %s", code, b)
	}
	if _, b = send(t, authed, "GET", u+"/system/info", ""); !strings.Contains(string(b), `"tmdb":true`) {
		t.Fatalf("tmdb live: %s", b)
	}
	if code, b = send(t, authed, "PATCH", u+"/settings", `{"maxTranscodes":99}`); code != 400 || !strings.Contains(string(b), "invalid_setting") {
		t.Fatalf("invalid: %d %s", code, b)
	}
	if code, _ = send(t, authed, "PATCH", u+"/settings", `{"unknown":1}`); code != 400 {
		t.Fatalf("unknown field: %d", code)
	}
	if code, b = send(t, authed, "PATCH", u+"/settings", `{"wizard":{"completed":true}}`); code != 200 {
		t.Fatalf("complete: %d %s", code, b)
	}
	if _, b = send(t, authed, "GET", u+"/system/info", ""); strings.Contains(string(b), "setupWizardPending") {
		t.Fatalf("still pending: %s", b)
	}
	if code, b = send(t, authed, "GET", u+"/system/status", ""); code != 200 || !strings.Contains(string(b), `"database"`) {
		t.Fatalf("status: %d %s", code, b)
	}
}

func TestJobsAPI(t *testing.T) {
	env := adminServer(t)
	u := env.ts.URL + "/api/v1"
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); env.runner.Wait() }()
	if err := env.runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := env.runner.Enqueue(ctx, "test.fail", "a", nil, 0); err != nil {
		t.Fatal(err)
	}
	var q jobQueueDTO
	waitFor(t, func() bool {
		_, b := send(t, authed, "GET", u+"/jobs/queue", "")
		_ = json.Unmarshal(b, &q)
		return len(q.Failed) == 1
	})
	if strings.Contains(q.Failed[0].Error, goodKey) || !strings.Contains(q.Failed[0].Error, "boom") {
		t.Fatalf("failed job error = %q", q.Failed[0].Error)
	}
	id := q.Failed[0].ID
	if code, b := send(t, authed, "POST", fmt.Sprintf("%s/jobs/%d/retry", u, id), ""); code != 204 {
		t.Fatalf("retry: %d %s", code, b)
	}
	if code, _ := send(t, authed, "POST", u+"/jobs/999/retry", ""); code != 404 {
		t.Fatalf("retry unknown: %d", code)
	}
	waitFor(t, func() bool {
		_, b := send(t, authed, "GET", u+"/jobs/queue", "")
		_ = json.Unmarshal(b, &q)
		return len(q.Failed) == 1 && q.Failed[0].Attempts == 1
	})
	if code, _ := send(t, authed, "DELETE", fmt.Sprintf("%s/jobs/%d", u, id), ""); code != 204 { // dismiss a failure
		t.Fatalf("dismiss: %d", code)
	}
	// Cancel a running job.
	if _, err := env.runner.Enqueue(ctx, "test.block", "b", nil, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		_, b := send(t, authed, "GET", u+"/jobs/queue", "")
		_ = json.Unmarshal(b, &q)
		return len(q.Running) == 1
	})
	if code, _ := send(t, authed, "DELETE", fmt.Sprintf("%s/jobs/%d", u, q.Running[0].ID), ""); code != 204 {
		t.Fatalf("cancel: %d", code)
	}
	waitFor(t, func() bool {
		_, b := send(t, authed, "GET", u+"/jobs/queue", "")
		_ = json.Unmarshal(b, &q)
		return q.Counts["running"] == 0 && q.Counts["queued"] == 0 && q.Counts["failed"] == 0
	})
	if code, b := send(t, authed, "POST", u+"/jobs/run", `{"task":"bogus"}`); code != 400 {
		t.Fatalf("bogus task: %d %s", code, b)
	}
}

func TestLogsAPI(t *testing.T) {
	env := adminServer(t)
	u := env.ts.URL + "/api/v1"
	env.log.Warn("source hors ligne", "source", 3, "token", "s3cr3t-token-value")
	env.log.Info("GET https://api.themoviedb.org/3/x?api_key=" + goodKey)
	code, b := send(t, authed, "GET", u+"/logs?level=warn&q=hors", "")
	if code != 200 || !strings.Contains(string(b), "source hors ligne") || strings.Contains(string(b), "api.themoviedb") {
		t.Fatalf("logs: %d %s", code, b)
	}
	code, b = send(t, authed, "GET", u+"/logs/download", "")
	if code != 200 || strings.Contains(string(b), goodKey) || strings.Contains(string(b), "s3cr3t-token-value") || !strings.Contains(string(b), "WARN") {
		t.Fatalf("download: %d %s", code, b)
	}
	// Live tail.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u+"/logs/stream?q=direct", nil)
	res, err := authed.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	go func() {
		time.Sleep(100 * time.Millisecond)
		env.log.Info("ignoré")
		env.log.Info("en direct", "n", 1)
	}()
	buf := make([]byte, 4096)
	var got strings.Builder
	for !strings.Contains(got.String(), "en direct") {
		n, err := res.Body.Read(buf)
		if err != nil {
			t.Fatalf("stream: %v (%q)", err, got.String())
		}
		got.Write(buf[:n])
	}
	if strings.Contains(got.String(), "ignoré") || !strings.Contains(got.String(), "event: log") {
		t.Fatalf("stream = %q", got.String())
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(errors.New("condition not met in time"))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
