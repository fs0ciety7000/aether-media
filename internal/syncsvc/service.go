package syncsvc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// Job kinds.
const (
	JobRun      = "sync.run"
	JobScrobble = "sync.scrobble"
)

// Services in display order.
var Services = []string{"trakt", "simkl"}

var (
	ErrUnknownService     = errors.New("service inconnu")
	ErrUnavailable        = errors.New("service non configuré par l'administrateur")
	ErrManagedByConfig    = errors.New("identifiants fournis par la configuration du serveur")
	ErrNotConnected       = errors.New("aucune connexion à ce service")
	ErrNotPending         = errors.New("aucun appairage en cours")
	ErrReauth             = errors.New("reconnexion nécessaire")
	ErrUnsupportedFeature = errors.New("fonction non prise en charge par ce service")
	ErrInvalidCreds       = errors.New("identifiants d'application incomplets")
)

// Options configure a Service.
type Options struct {
	// Credentials from the configuration file / environment (take precedence
	// over those stored by an administrator). Empty ClientID = none.
	Config map[string]Creds
	// Interval between periodic synchronisations (default 30 min).
	Interval time.Duration
	// LocalDelay debounces the synchronisation that follows local changes
	// (default 1 min; scrobble jobs get the time to land first).
	LocalDelay time.Duration
}

// Service owns the per-user links and the synchronisation jobs.
type Service struct {
	store     *store.Store
	box       *secret.Box
	runner    *jobs.Runner
	hub       *events.Hub
	log       *slog.Logger
	providers map[string]Provider
	opts      Options

	mu    sync.Mutex
	locks map[int64]*sync.Mutex // per connection: token refresh, merges
	now   func() time.Time
}

// New builds the service. hub may be nil.
func New(st *store.Store, box *secret.Box, runner *jobs.Runner, hub *events.Hub, log *slog.Logger, opts Options, providers ...Provider) *Service {
	if opts.Interval <= 0 {
		opts.Interval = 30 * time.Minute
	}
	if opts.LocalDelay < 0 {
		opts.LocalDelay = 0
	} else if opts.LocalDelay == 0 {
		opts.LocalDelay = time.Minute
	}
	s := &Service{store: st, box: box, runner: runner, hub: hub, log: log, providers: map[string]Provider{},
		opts: opts, locks: map[int64]*sync.Mutex{}, now: time.Now}
	for _, p := range providers {
		s.providers[p.Service()] = p
	}
	return s
}

// Register declares the job handlers (before runner.Start).
func (s *Service) Register() {
	s.runner.Register(JobRun, 2, s.handleRun)
	s.runner.Register(JobScrobble, 1, s.handleScrobble)
}

// Run schedules the periodic synchronisation of every active link and
// purges old journal entries, until ctx is done.
func (s *Service) Run(ctx context.Context) {
	first := time.NewTimer(time.Minute)
	defer first.Stop()
	tick := time.NewTicker(s.opts.Interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
		case <-tick.C:
		}
		s.EnqueueAll(ctx)
	}
}

// EnqueueAll queues a synchronisation of every active link and returns
// the number of synchronisations queued.
func (s *Service) EnqueueAll(ctx context.Context) int {
	conns, err := s.store.ListActiveSyncConnections(ctx)
	if err != nil {
		s.log.Error("synchronisation périodique", "err", err)
		return 0
	}
	n := 0
	for _, c := range conns {
		ok, err := s.enqueueRun(ctx, c.ID, false, 0)
		if err != nil {
			s.log.Error("synchronisation périodique", "connection", c.ID, "err", err)
		}
		if ok {
			n++
		}
	}
	if _, err := s.store.PurgeSyncEvents(ctx, s.now().Add(-30*24*time.Hour).Unix()); err != nil {
		s.log.Warn("purge du journal de synchronisation", "err", err)
	}
	return n
}

func (s *Service) provider(service string) (Provider, error) {
	p, ok := s.providers[service]
	if !ok {
		return nil, ErrUnknownService
	}
	return p, nil
}

func (s *Service) lock(connID int64) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.locks[connID]
	if !ok {
		m = &sync.Mutex{}
		s.locks[connID] = m
	}
	return m
}

// ---- application credentials ----

// AppStatus describes the application credentials of a service.
type AppStatus struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source"` // config | database | none
	ClientID   string `json:"clientId,omitempty"`
}

func (s *Service) creds(ctx context.Context, service string) (Creds, string, error) {
	p, err := s.provider(service)
	if err != nil {
		return Creds{}, "", err
	}
	complete := func(c Creds) bool { return c.ClientID != "" && (!p.NeedsSecret() || c.ClientSecret != "") }
	if c := s.opts.Config[service]; c.ClientID != "" {
		if !complete(c) {
			return Creds{}, "config", ErrUnavailable
		}
		return c, "config", nil
	}
	row, err := s.store.GetSyncApp(ctx, service)
	if errors.Is(err, sql.ErrNoRows) {
		return Creds{}, "none", ErrUnavailable
	}
	if err != nil {
		return Creds{}, "", err
	}
	var c Creds
	if err := s.box.OpenJSON(row.Secret, &c); err != nil {
		return Creds{}, "database", err
	}
	if !complete(c) {
		return Creds{}, "database", ErrUnavailable
	}
	return c, "database", nil
}

// Available reports whether users can link this service.
func (s *Service) Available(ctx context.Context, service string) bool {
	_, _, err := s.creds(ctx, service)
	return err == nil
}

func mask(id string) string {
	if len(id) <= 8 {
		return "…"
	}
	return id[:4] + "…" + id[len(id)-4:]
}

// Apps returns the credential status of every service.
func (s *Service) Apps(ctx context.Context) (map[string]AppStatus, error) {
	out := map[string]AppStatus{}
	for _, name := range Services {
		c, src, err := s.creds(ctx, name)
		if err != nil && !errors.Is(err, ErrUnavailable) {
			return nil, err
		}
		st := AppStatus{Configured: err == nil, Source: src}
		if src == "" {
			st.Source = "none"
		}
		if c.ClientID != "" {
			st.ClientID = mask(c.ClientID)
		}
		out[name] = st
	}
	return out, nil
}

// SetApp stores application credentials entered by an administrator.
func (s *Service) SetApp(ctx context.Context, service string, c Creds) error {
	p, err := s.provider(service)
	if err != nil {
		return err
	}
	if s.opts.Config[service].ClientID != "" {
		return ErrManagedByConfig
	}
	if c.ClientID == "" || (p.NeedsSecret() && c.ClientSecret == "") || len(c.ClientID) > 256 || len(c.ClientSecret) > 256 {
		return ErrInvalidCreds
	}
	if !p.NeedsSecret() {
		c.ClientSecret = ""
	}
	sealed, err := s.box.SealJSON(c)
	if err != nil {
		return err
	}
	return s.store.UpsertSyncApp(ctx, db.UpsertSyncAppParams{Service: service, Secret: sealed})
}

// DeleteApp forgets credentials stored by an administrator.
func (s *Service) DeleteApp(ctx context.Context, service string) error {
	if _, err := s.provider(service); err != nil {
		return err
	}
	if s.opts.Config[service].ClientID != "" {
		return ErrManagedByConfig
	}
	return s.store.DeleteSyncApp(ctx, service)
}

// ---- connections ----

// Connection is a user's link as shown by the API (no secret).
type Connection struct {
	Service     string           `json:"service"`
	Available   bool             `json:"available"`
	Status      string           `json:"status"`
	Account     *AccountView     `json:"account,omitempty"`
	Supported   []Feature        `json:"supported"`
	Features    map[Feature]bool `json:"features"`
	Syncing     bool             `json:"syncing"`
	ConnectedAt *time.Time       `json:"connectedAt,omitempty"`
	LastSyncAt  *time.Time       `json:"lastSyncAt,omitempty"`
	LastError   string           `json:"lastError,omitempty"`
	LastErrorAt *time.Time       `json:"lastErrorAt,omitempty"`
	Pending     *Pending         `json:"pending,omitempty"`
}

type AccountView struct {
	Username string `json:"username"`
}

// Pending is a pairing in progress.
type Pending struct {
	UserCode        string    `json:"userCode"`
	VerificationURL string    `json:"verificationUrl"`
	ExpiresAt       time.Time `json:"expiresAt"`
	Interval        int64     `json:"interval"`
}

func tptr(t sql.NullInt64) *time.Time {
	if !t.Valid || t.Int64 <= 0 {
		return nil
	}
	v := time.Unix(t.Int64, 0).UTC()
	return &v
}

func (s *Service) supported(p Provider) []Feature {
	var out []Feature
	for _, f := range AllFeatures {
		if p.Supports(f) {
			out = append(out, f)
		}
	}
	return out
}

func featureMap(raw string) map[Feature]bool {
	m := map[Feature]bool{}
	_ = json.Unmarshal([]byte(raw), &m)
	return m
}

// enabled reports whether a feature is on for a connection.
func (s *Service) enabled(p Provider, c db.SyncConnection, f Feature) bool {
	return p.Supports(f) && featureMap(c.Features)[f]
}

func (s *Service) view(ctx context.Context, p Provider, c *db.SyncConnection) (Connection, error) {
	v := Connection{Service: p.Service(), Available: s.Available(ctx, p.Service()), Status: "disconnected",
		Supported: s.supported(p), Features: map[Feature]bool{}}
	for _, f := range v.Supported {
		v.Features[f] = true // defaults shown before connecting
	}
	if c == nil {
		return v, nil
	}
	if c.Secret != nil {
		v.Status = c.Status
		if v.Status == "pending" {
			v.Status = "connected"
		}
		fm := featureMap(c.Features)
		for _, f := range v.Supported {
			v.Features[f] = fm[f]
		}
		if c.Account != "" {
			v.Account = &AccountView{Username: c.Account}
		}
	}
	if c.PendingUserCode != "" && c.PendingExpiresAt.Valid && c.PendingExpiresAt.Int64 > s.now().Unix() {
		v.Pending = &Pending{UserCode: c.PendingUserCode, VerificationURL: c.PendingUrl,
			ExpiresAt: time.Unix(c.PendingExpiresAt.Int64, 0).UTC(), Interval: c.PendingInterval}
		if c.Secret == nil {
			v.Status = "pending"
		}
	}
	n, err := s.store.CountActiveJobs(ctx, db.CountActiveJobsParams{Kind: JobRun, Key: runKey(c.ID)})
	if err != nil {
		return v, err
	}
	v.Syncing = n > 0
	v.ConnectedAt, v.LastSyncAt, v.LastErrorAt = tptr(c.ConnectedAt), tptr(c.LastSyncAt), tptr(c.LastErrorAt)
	v.LastError = c.LastError
	return v, nil
}

func (s *Service) load(ctx context.Context, userID int64, service string) (*db.SyncConnection, error) {
	c, err := s.store.GetSyncConnection(ctx, db.GetSyncConnectionParams{UserID: userID, Service: service})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Connections returns the user's links to every service.
func (s *Service) Connections(ctx context.Context, userID int64) ([]Connection, error) {
	out := make([]Connection, 0, len(Services))
	for _, name := range Services {
		c, err := s.Connection(ctx, userID, name)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// Connection returns the user's link to one service.
func (s *Service) Connection(ctx context.Context, userID int64, service string) (Connection, error) {
	p, err := s.provider(service)
	if err != nil {
		return Connection{}, err
	}
	c, err := s.load(ctx, userID, service)
	if err != nil {
		return Connection{}, err
	}
	return s.view(ctx, p, c)
}

// Connect starts a device code (Trakt) or PIN (Simkl) pairing.
func (s *Service) Connect(ctx context.Context, userID int64, service string) (Pending, error) {
	p, err := s.provider(service)
	if err != nil {
		return Pending{}, err
	}
	creds, _, err := s.creds(ctx, service)
	if err != nil {
		return Pending{}, err
	}
	da, err := p.StartAuth(ctx, creds)
	if err != nil {
		return Pending{}, err
	}
	sealed, err := s.box.SealJSON(map[string]string{"deviceCode": da.DeviceCode})
	if err != nil {
		return Pending{}, err
	}
	exp := s.now().Add(da.ExpiresIn)
	interval := max(int64(da.Interval/time.Second), 1)
	if _, err := s.store.UpsertSyncPending(ctx, db.UpsertSyncPendingParams{
		UserID: userID, Service: service, PendingSecret: sealed, PendingUserCode: da.UserCode,
		PendingUrl: da.VerificationURL, PendingExpiresAt: store.NullTime(exp.Unix()), PendingInterval: interval,
	}); err != nil {
		return Pending{}, err
	}
	return Pending{UserCode: da.UserCode, VerificationURL: da.VerificationURL, ExpiresAt: exp.UTC().Truncate(time.Second), Interval: interval}, nil
}

// PollResult is the outcome of one poll of a pairing.
type PollResult struct {
	Status     string     `json:"status"` // pending | slow_down | connected | expired | denied
	Interval   int64      `json:"interval"`
	Connection Connection `json:"connection"`
}

// Poll checks whether the user approved the pairing code. Calls closer
// than the advertised interval do not reach the service.
func (s *Service) Poll(ctx context.Context, userID int64, service string) (PollResult, error) {
	p, err := s.provider(service)
	if err != nil {
		return PollResult{}, err
	}
	c, err := s.load(ctx, userID, service)
	if err != nil {
		return PollResult{}, err
	}
	if c == nil || c.PendingSecret == nil {
		return PollResult{}, ErrNotPending
	}
	res := PollResult{Status: "pending", Interval: c.PendingInterval}
	finish := func() (PollResult, error) {
		cc, err := s.load(ctx, userID, service)
		if err != nil {
			return res, err
		}
		res.Connection, err = s.view(ctx, p, cc)
		return res, err
	}
	now := s.now()
	if !c.PendingExpiresAt.Valid || now.Unix() >= c.PendingExpiresAt.Int64 {
		res.Status = "expired"
		if err := s.store.ClearSyncPending(ctx, c.ID); err != nil {
			return res, err
		}
		return finish()
	}
	if c.PendingPolledAt.Valid && now.Unix() < c.PendingPolledAt.Int64+c.PendingInterval {
		return finish()
	}
	creds, _, err := s.creds(ctx, service)
	if err != nil {
		return res, err
	}
	var pend struct {
		DeviceCode string `json:"deviceCode"`
	}
	if err := s.box.OpenJSON(c.PendingSecret, &pend); err != nil {
		return res, err
	}
	tok, err := p.PollAuth(ctx, creds, pend.DeviceCode)
	switch {
	case errors.Is(err, ErrAuthPending):
		err = s.store.SetSyncPendingPoll(ctx, db.SetSyncPendingPollParams{PendingInterval: c.PendingInterval, PendingPolledAt: store.NullTime(now.Unix()), ID: c.ID})
		if err != nil {
			return res, err
		}
		return finish()
	case errors.Is(err, ErrSlowDown):
		res.Status, res.Interval = "slow_down", c.PendingInterval+5
		err = s.store.SetSyncPendingPoll(ctx, db.SetSyncPendingPollParams{PendingInterval: res.Interval, PendingPolledAt: store.NullTime(now.Unix()), ID: c.ID})
		if err != nil {
			return res, err
		}
		return finish()
	case errors.Is(err, ErrAuthExpired), errors.Is(err, ErrAuthDenied):
		res.Status = "expired"
		if errors.Is(err, ErrAuthDenied) {
			res.Status = "denied"
		}
		if err := s.store.ClearSyncPending(ctx, c.ID); err != nil {
			return res, err
		}
		return finish()
	case err != nil:
		return res, err
	}
	acct, err := p.Account(ctx, creds, tok.Access)
	if err != nil {
		return res, err
	}
	if err := s.saveConnected(ctx, p, c, tok, acct); err != nil {
		return res, err
	}
	res.Status = "connected"
	s.log.Info("synchronisation connectée", "service", service, "user", userID)
	return finish()
}

func (s *Service) saveConnected(ctx context.Context, p Provider, c *db.SyncConnection, tok Token, acct Account) error {
	sealed, err := s.box.SealJSON(tok)
	if err != nil {
		return err
	}
	err = s.store.Tx(ctx, func(q *db.Queries) error {
		if c.RemoteID != "" && c.RemoteID != acct.ID {
			// Another account: what was synchronised before no longer applies.
			if err := q.ClearSyncState(ctx, c.ID); err != nil {
				return err
			}
			if err := q.SetSyncCursors(ctx, db.SetSyncCursorsParams{Cursors: "{}", ID: c.ID}); err != nil {
				return err
			}
		}
		if c.Secret == nil && c.Features == "{}" {
			fm := map[Feature]bool{}
			for _, f := range s.supported(p) {
				fm[f] = true
			}
			b, _ := json.Marshal(fm)
			if err := q.SetSyncFeatures(ctx, db.SetSyncFeaturesParams{Features: string(b), ID: c.ID}); err != nil {
				return err
			}
		}
		return q.SetSyncConnected(ctx, db.SetSyncConnectedParams{
			Secret: sealed, TokenExpiresAt: expiryCol(tok), Account: acct.Username, RemoteID: acct.ID, ID: c.ID,
		})
	})
	if err != nil {
		return err
	}
	_, err = s.enqueueRun(ctx, c.ID, true, 0)
	s.publish(ctx, c.ID)
	return err
}

func expiryCol(t Token) sql.NullInt64 {
	if t.Expiry.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.Expiry.Unix(), Valid: true}
}

// Disconnect revokes the token (best effort) and forgets the link, its
// synchronisation state and journal. Local data is kept.
func (s *Service) Disconnect(ctx context.Context, userID int64, service string) error {
	p, err := s.provider(service)
	if err != nil {
		return err
	}
	c, err := s.load(ctx, userID, service)
	if err != nil {
		return err
	}
	if c == nil {
		return ErrNotConnected
	}
	if c.Secret != nil {
		var tok Token
		if err := s.box.OpenJSON(c.Secret, &tok); err == nil {
			if creds, _, err := s.creds(ctx, service); err == nil {
				rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				if err := p.Revoke(rctx, creds, tok); err != nil {
					s.log.Warn("révocation du jeton", "service", service, "err", err)
				}
				cancel()
			}
		}
	}
	if err := s.store.DeleteSyncConnection(ctx, c.ID); err != nil {
		return err
	}
	s.hubPublish(userID, map[string]any{"userId": userID, "service": service, "status": "disconnected", "syncing": false})
	return nil
}

// SetFeatures switches functions on or off. A function switched back on
// starts again from an initial import (no removals inferred).
func (s *Service) SetFeatures(ctx context.Context, userID int64, service string, set map[Feature]bool) (Connection, error) {
	p, err := s.provider(service)
	if err != nil {
		return Connection{}, err
	}
	c, err := s.load(ctx, userID, service)
	if err != nil {
		return Connection{}, err
	}
	if c == nil || c.Secret == nil {
		return Connection{}, ErrNotConnected
	}
	fm := featureMap(c.Features)
	cursors := cursorMap(c.Cursors)
	var turnedOn []Feature
	for f, on := range set {
		if !slices.Contains(AllFeatures, f) || (on && !p.Supports(f)) {
			return Connection{}, fmt.Errorf("%w: %s", ErrUnsupportedFeature, f)
		}
		if on && !fm[f] {
			turnedOn = append(turnedOn, f)
			delete(cursors, f)
		}
		fm[f] = on
	}
	b, _ := json.Marshal(fm)
	cb, _ := json.Marshal(cursors)
	err = s.store.Tx(ctx, func(q *db.Queries) error {
		for _, f := range turnedOn {
			if err := q.ClearSyncStateFeature(ctx, db.ClearSyncStateFeatureParams{ConnectionID: c.ID, Feature: string(f)}); err != nil {
				return err
			}
		}
		if err := q.SetSyncCursors(ctx, db.SetSyncCursorsParams{Cursors: string(cb), ID: c.ID}); err != nil {
			return err
		}
		return q.SetSyncFeatures(ctx, db.SetSyncFeaturesParams{Features: string(b), ID: c.ID})
	})
	if err != nil {
		return Connection{}, err
	}
	if len(turnedOn) > 0 && c.Status == "connected" {
		if _, err := s.enqueueRun(ctx, c.ID, false, 0); err != nil {
			return Connection{}, err
		}
	}
	return s.Connection(ctx, userID, service)
}

// RunNow queues a synchronisation ("synchroniser maintenant").
func (s *Service) RunNow(ctx context.Context, userID int64, service string, full bool) (bool, error) {
	if _, err := s.provider(service); err != nil {
		return false, err
	}
	c, err := s.load(ctx, userID, service)
	if err != nil {
		return false, err
	}
	if c == nil || c.Secret == nil {
		return false, ErrNotConnected
	}
	if c.Status == "reauth_required" {
		return false, ErrReauth
	}
	return s.enqueueRun(ctx, c.ID, full, 0)
}

// Event is a journal entry.
type Event struct {
	ID        int64     `json:"id"`
	Time      time.Time `json:"time"`
	Kind      string    `json:"kind"`
	Feature   string    `json:"feature"`
	ItemID    *int64    `json:"itemId,omitempty"`
	ItemTitle string    `json:"itemTitle,omitempty"`
	Winner    string    `json:"winner,omitempty"`
	Message   string    `json:"message"`
}

// Events returns the journal of the user's link, most recent first.
func (s *Service) Events(ctx context.Context, userID int64, service string, limit int) ([]Event, error) {
	if _, err := s.provider(service); err != nil {
		return nil, err
	}
	c, err := s.load(ctx, userID, service)
	if err != nil {
		return nil, err
	}
	out := []Event{}
	if c == nil {
		return out, nil
	}
	rows, err := s.store.ListSyncEvents(ctx, db.ListSyncEventsParams{ConnectionID: c.ID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		e := Event{ID: r.ID, Time: time.Unix(r.CreatedAt, 0).UTC(), Kind: r.Kind, Feature: r.Feature,
			ItemTitle: r.ItemTitle, Winner: r.Winner, Message: r.Message}
		if r.ItemID.Valid {
			id := r.ItemID.Int64
			e.ItemID = &id
		}
		out = append(out, e)
	}
	return out, nil
}

// ---- tokens ----

// withAccess runs fn with a valid access token, refreshing it ahead of
// expiry or once after a 401. A refused refresh marks the link
// reauth_required and returns ErrReauth.
func (s *Service) withAccess(ctx context.Context, connID int64, fn func(p Provider, c Creds, access string) error) error {
	conn, err := s.store.GetSyncConnectionByID(ctx, connID)
	if err != nil {
		return err
	}
	p, err := s.provider(conn.Service)
	if err != nil {
		return err
	}
	if conn.Status == "reauth_required" || conn.Secret == nil {
		return ErrReauth
	}
	creds, _, err := s.creds(ctx, conn.Service)
	if err != nil {
		return err
	}
	tok, err := s.token(ctx, p, creds, connID, false)
	if err != nil {
		return err
	}
	err = fn(p, creds, tok.Access)
	if !errors.Is(err, ErrUnauthorized) {
		return err
	}
	if tok.Refresh == "" {
		return s.reauth(ctx, connID, err)
	}
	if tok, err = s.token(ctx, p, creds, connID, true); err != nil {
		return err
	}
	err = fn(p, creds, tok.Access)
	if errors.Is(err, ErrUnauthorized) {
		return s.reauth(ctx, connID, err)
	}
	return err
}

// token returns the stored token, refreshed when it expires within the
// hour (or when force is set).
func (s *Service) token(ctx context.Context, p Provider, creds Creds, connID int64, force bool) (Token, error) {
	m := s.lock(connID)
	m.Lock()
	defer m.Unlock()
	conn, err := s.store.GetSyncConnectionByID(ctx, connID) // fresh: another worker may have refreshed
	if err != nil {
		return Token{}, err
	}
	var tok Token
	if err := s.box.OpenJSON(conn.Secret, &tok); err != nil {
		return Token{}, err
	}
	if conn.TokenExpiresAt.Valid {
		tok.Expiry = time.Unix(conn.TokenExpiresAt.Int64, 0)
	}
	if !force && (tok.Expiry.IsZero() || tok.Expiry.Sub(s.now()) > time.Hour) {
		return tok, nil
	}
	if tok.Refresh == "" {
		if force || s.now().After(tok.Expiry) {
			return Token{}, s.reauth(ctx, connID, ErrUnauthorized)
		}
		return tok, nil
	}
	nt, err := p.Refresh(ctx, creds, tok)
	if errors.Is(err, ErrUnauthorized) {
		return Token{}, s.reauth(ctx, connID, err)
	}
	if err != nil {
		return Token{}, err
	}
	if nt.Refresh == "" {
		nt.Refresh = tok.Refresh
	}
	sealed, err := s.box.SealJSON(nt)
	if err != nil {
		return Token{}, err
	}
	if err := s.store.SetSyncTokens(ctx, db.SetSyncTokensParams{Secret: sealed, TokenExpiresAt: expiryCol(nt), ID: connID}); err != nil {
		return Token{}, err
	}
	s.log.Info("jeton de synchronisation renouvelé", "service", p.Service(), "connection", connID)
	return nt, nil
}

func (s *Service) reauth(ctx context.Context, connID int64, cause error) error {
	bg := context.WithoutCancel(ctx)
	if err := s.store.SetSyncReauth(bg, db.SetSyncReauthParams{LastError: "Accès refusé par le service : reconnectez le compte.", ID: connID}); err != nil {
		s.log.Error("synchronisation", "err", err)
	}
	s.publish(bg, connID)
	return fmt.Errorf("%w: %w", ErrReauth, cause)
}

// ---- jobs & events ----

func runKey(connID int64) string { return "conn:" + itoa(connID) }

type runPayload struct {
	Connection int64 `json:"connection"`
	Full       bool  `json:"full"`
}

func (s *Service) enqueueRun(ctx context.Context, connID int64, full bool, delay time.Duration) (bool, error) {
	return s.runner.EnqueueAfter(ctx, JobRun, runKey(connID), runPayload{Connection: connID, Full: full}, 0, delay)
}

// LocalChanged is called after the user changed watched state, ratings,
// resume points or the watchlist: their links are synchronised shortly
// after (changes made in a burst are merged into one run). except skips
// one connection (the one an import came from).
func (s *Service) LocalChanged(ctx context.Context, userID int64) { s.localChanged(ctx, userID, 0) }

func (s *Service) localChanged(ctx context.Context, userID, except int64) {
	conns, err := s.store.ListUserSyncConnections(ctx, userID)
	if err != nil {
		s.log.Error("synchronisation", "err", err)
		return
	}
	for _, c := range conns {
		if c.ID == except || c.Status != "connected" || c.Secret == nil {
			continue
		}
		if _, err := s.enqueueRun(ctx, c.ID, false, s.opts.LocalDelay); err != nil {
			s.log.Error("synchronisation", "connection", c.ID, "err", err)
		}
	}
}

func (s *Service) hubPublish(userID int64, data map[string]any) {
	if s.hub != nil {
		s.hub.PublishUser(userID, "sync.status", data)
	}
}

// publish announces the state of a link (no secret).
func (s *Service) publish(ctx context.Context, connID int64) {
	if s.hub == nil {
		return
	}
	c, err := s.store.GetSyncConnectionByID(ctx, connID)
	if err != nil {
		return
	}
	p, err := s.provider(c.Service)
	if err != nil {
		return
	}
	v, err := s.view(ctx, p, &c)
	if err != nil {
		return
	}
	data := map[string]any{"userId": c.UserID, "service": c.Service, "status": v.Status, "syncing": v.Syncing}
	if v.LastError != "" {
		data["lastError"] = v.LastError
	}
	s.hubPublish(c.UserID, data)
}

// jobError turns a provider error into a job outcome: rate limits wait for
// Retry-After, refused access and unknown items are permanent, transient
// failures retry with backoff. The user-facing message is recorded.
func (s *Service) jobError(ctx context.Context, connID int64, err error) error {
	if err == nil || ctx.Err() != nil {
		return err
	}
	bg := context.WithoutCancel(ctx)
	if !errors.Is(err, ErrReauth) {
		if e := s.store.SetSyncError(bg, db.SetSyncErrorParams{LastError: userMessage(err), ID: connID}); e != nil {
			s.log.Error("synchronisation", "err", e)
		}
		s.publish(bg, connID)
	}
	var rl *RateLimitError
	var re *RemoteError
	switch {
	case errors.As(err, &rl):
		return jobs.RetryAfter(err, rl.RetryAfter)
	case errors.Is(err, ErrReauth), errors.Is(err, ErrNotFound), errors.Is(err, ErrUnavailable), errors.Is(err, ErrNotSupported):
		return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
	case errors.As(err, &re) && !re.Temporary():
		return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
	}
	return err
}

// userMessage is the error shown to the user (never contains a secret:
// provider errors are built without request bodies or tokens).
func userMessage(err error) string {
	var rl *RateLimitError
	var re *RemoteError
	switch {
	case errors.As(err, &rl):
		return "Limite de requêtes atteinte : nouvel essai automatique dans " + rl.RetryAfter.Round(time.Second).String() + "."
	case errors.Is(err, ErrReauth):
		return "Accès refusé par le service : reconnectez le compte."
	case errors.Is(err, ErrUnavailable):
		return "Service non configuré par l'administrateur."
	case errors.As(err, &re):
		return "Erreur du service : " + re.Error()
	}
	return "Erreur interne de synchronisation."
}
