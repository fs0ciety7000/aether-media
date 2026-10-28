package syncsvc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

const (
	// fullEvery forces a complete fetch now and then even when the remote
	// activity did not move (items added to the library since).
	fullEvery = 24 * time.Hour
	// maxProgressPush caps resume points sent per run (one request each).
	maxProgressPush = 50
	maxConflictLogs = 50
	fullCursor      = "_full"
)

var featureLabel = map[Feature]string{
	FeatHistory: "historique", FeatProgress: "reprise", FeatWatchlist: "watchlist", FeatRatings: "notes", FeatScrobble: "scrobbling",
}

func cursorMap(raw string) map[Feature]time.Time {
	var m map[Feature]string
	out := map[Feature]time.Time{}
	if json.Unmarshal([]byte(raw), &m) != nil {
		return out
	}
	for k, v := range m {
		if t := parseTime(v); !t.IsZero() {
			out[k] = t
		}
	}
	return out
}

func encodeCursors(m map[Feature]time.Time) string {
	out := map[Feature]string{}
	for k, v := range m {
		if !v.IsZero() {
			out[k] = v.UTC().Format(time.RFC3339Nano)
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// handleRun synchronises one link (job JobRun).
func (s *Service) handleRun(ctx context.Context, j jobs.Job) error {
	var pl runPayload
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
	}
	conn, err := s.store.GetSyncConnectionByID(ctx, pl.Connection)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // disconnected meanwhile
	}
	if err != nil {
		return err
	}
	if conn.Status != "connected" || conn.Secret == nil {
		return nil
	}
	p, err := s.provider(conn.Service)
	if err != nil {
		return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
	}
	var features []Feature
	for _, f := range mergeFeatures {
		if s.enabled(p, conn, f) {
			features = append(features, f)
		}
	}
	if len(features) > 0 {
		err = s.withAccess(ctx, conn.ID, func(p Provider, c Creds, access string) error {
			return s.syncConn(ctx, p, c, access, conn, features, pl.Full)
		})
		if err != nil {
			s.log.Warn("synchronisation en échec", "service", conn.Service, "connection", conn.ID, "err", err)
			return s.jobError(ctx, conn.ID, err)
		}
	}
	bg := context.WithoutCancel(ctx)
	if err := s.store.SetSyncSuccess(bg, conn.ID); err != nil {
		return err
	}
	go s.publish(bg, conn.ID) // after the job is gone, so "syncing" reads false
	return nil
}

func (s *Service) syncConn(ctx context.Context, p Provider, creds Creds, access string, conn db.SyncConnection, features []Feature, full bool) error {
	viewer, err := s.store.ViewerFor(ctx, conn.UserID)
	if err != nil {
		return err
	}
	items, err := s.store.SyncItems(ctx, viewer)
	if err != nil {
		return err
	}
	ix := newIndex(items)
	acts, err := p.Activities(ctx, creds, access)
	if err != nil {
		return err
	}
	cursors := cursorMap(conn.Cursors)
	now := s.now()
	if now.Sub(cursors[fullCursor]) > fullEvery {
		full = true
	}
	shadows := map[Feature]map[int64]state{}
	var fetch []Feature
	for _, f := range features {
		rows, err := s.store.ListSyncState(ctx, db.ListSyncStateParams{ConnectionID: conn.ID, Feature: string(f)})
		if err != nil {
			return err
		}
		sh := map[int64]state{}
		for _, r := range rows {
			sh[r.ItemID] = state{present: true, at: unix(r.RemoteAt.Int64), value: r.Value, ref: r.RemoteRef}
		}
		shadows[f] = sh
		a, cur := acts[f], cursors[f]
		if full || a.IsZero() || cur.IsZero() || a.After(cur) {
			fetch = append(fetch, f)
		}
	}
	remote := map[Feature][]Entry{}
	if len(fetch) > 0 {
		if remote, err = p.Fetch(ctx, creds, access, fetch); err != nil {
			return err
		}
	}
	changed := map[int64]bool{}
	for _, f := range features {
		var r map[int64]state
		if entries, ok := remote[f]; ok {
			r = remoteState(ix, p, f, entries)
		} else {
			r = shadows[f] // nothing moved remotely since the last run
		}
		l, err := s.localState(ctx, conn.UserID, ix, p, f)
		if err != nil {
			return err
		}
		eq := map[Feature]func(a, b float64) bool{FeatRatings: ratingEqual, FeatProgress: progressEqual}[f]
		pl := merge(l, r, shadows[f], acts[f], eq)
		ids, err := s.apply(ctx, p, creds, access, conn, ix, f, pl)
		if err != nil {
			return err
		}
		for _, id := range ids {
			changed[id] = true
		}
		if _, ok := remote[f]; ok && !acts[f].IsZero() {
			cursors[f] = acts[f]
		}
	}
	if full {
		cursors[fullCursor] = now
	}
	if err := s.store.SetSyncCursors(ctx, db.SetSyncCursorsParams{Cursors: encodeCursors(cursors), ID: conn.ID}); err != nil {
		return err
	}
	if len(changed) > 0 {
		if s.hub != nil {
			for id := range changed {
				s.hub.Publish("userdata.changed", map[string]any{"userId": conn.UserID, "itemId": id})
			}
		}
		// Imported changes travel on to the user's other links.
		s.localChanged(ctx, conn.UserID, conn.ID)
	}
	return nil
}

// remoteState maps remote entries onto local items.
func remoteState(ix *index, p Provider, f Feature, entries []Entry) map[int64]state {
	out := map[int64]state{}
	for _, e := range entries {
		if !p.Handles(f, e.Kind) {
			continue
		}
		id, ok := ix.match(e.Ref)
		if !ok {
			continue
		}
		if f == FeatProgress && ix.items[id].RuntimeMs <= 0 {
			continue
		}
		st := state{present: true, at: e.At, value: e.Value, ref: e.RemoteRef}
		if f == FeatRatings {
			st.value = toRemoteRating(e.Value)
		}
		if old, dup := out[id]; dup { // multi-episode file: keep the latest
			if old.at.After(st.at) {
				st.at = old.at
			}
			st.value = math.Max(st.value, old.value)
			st.ref = firstNonEmpty(old.ref, st.ref)
		}
		out[id] = st
	}
	return out
}

func first(ts ...sql.NullInt64) time.Time {
	for _, t := range ts {
		if t.Valid && t.Int64 > 0 {
			return unix(t.Int64)
		}
	}
	return time.Time{}
}

// localState reads the user's data for a feature, restricted to items the
// service can identify.
func (s *Service) localState(ctx context.Context, userID int64, ix *index, p Provider, f Feature) (map[int64]state, error) {
	out := map[int64]state{}
	usable := func(id int64) bool {
		_, ok := ix.items[id]
		return ok && p.Handles(f, ix.refKind(id)) && len(ix.refs(id)) > 0
	}
	if f == FeatWatchlist {
		rows, err := s.store.ListUserWatchlist(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if !usable(r.ItemID) {
				continue
			}
			if r.RemovedAt.Valid {
				out[r.ItemID] = state{at: unix(r.RemovedAt.Int64)}
			} else {
				out[r.ItemID] = state{present: true, at: unix(r.AddedAt)}
			}
		}
		return out, nil
	}
	rows, err := s.store.ListUserItemData(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if !usable(r.ItemID) {
			continue
		}
		updated := sql.NullInt64{Int64: r.UpdatedAt, Valid: true}
		switch f {
		case FeatHistory:
			if r.Played == 1 {
				out[r.ItemID] = state{present: true, at: first(r.PlayedChangedAt, r.LastPlayedAt, updated)}
			} else if r.PlayedChangedAt.Valid {
				out[r.ItemID] = state{at: unix(r.PlayedChangedAt.Int64)}
			}
		case FeatProgress:
			rt := ix.items[r.ItemID].RuntimeMs
			if rt <= 0 {
				continue
			}
			if r.PositionMs > 0 {
				out[r.ItemID] = state{present: true, at: first(r.LastPlayedAt, updated), value: math.Min(100, float64(r.PositionMs)*100/float64(rt))}
			} else {
				out[r.ItemID] = state{at: unix(r.UpdatedAt)}
			}
		case FeatRatings:
			if r.Rating.Valid {
				out[r.ItemID] = state{present: true, at: first(r.RatedAt, updated), value: toRemoteRating(r.Rating.Float64)}
			} else if r.RatedAt.Valid {
				out[r.ItemID] = state{at: unix(r.RatedAt.Int64)}
			}
		}
	}
	return out, nil
}

func orNow(t, now time.Time) time.Time {
	if t.IsZero() {
		return now
	}
	return t
}

func nullUnix(t time.Time) sql.NullInt64 { return store.NullTime(t.Unix()) }

// apply carries out a merge plan: remote changes first (nothing is recorded
// if they fail), then local writes, the new shadow and the journal, in one
// transaction. It returns the local items it modified.
func (s *Service) apply(ctx context.Context, p Provider, creds Creds, access string, conn db.SyncConnection, ix *index, f Feature, pl plan) ([]int64, error) {
	now := s.now().UTC()
	// A finished playback about to be scrobbled must not also be pushed to
	// the history (it would count twice).
	if f == FeatHistory {
		for id := range pl.pushAdd {
			n, err := s.store.CountActiveJobsLike(ctx, db.CountActiveJobsLikeParams{Kind: JobScrobble, Key: scrobblePrefix(conn.ID, id) + "%"})
			if err != nil {
				return nil, err
			}
			if n > 0 {
				delete(pl.pushAdd, id)
				delete(pl.shadow, id)
			}
		}
	}
	if f == FeatProgress && len(pl.pushAdd) > maxProgressPush {
		ids := make([]int64, 0, len(pl.pushAdd))
		for id := range pl.pushAdd {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(a, b int) bool { return pl.pushAdd[ids[a]].at.After(pl.pushAdd[ids[b]].at) })
		for _, id := range ids[maxProgressPush:] { // next run
			delete(pl.pushAdd, id)
			delete(pl.shadow, id)
		}
	}
	var adds, removes []Entry
	for id, st := range pl.pushAdd {
		for _, r := range ix.refs(id) {
			adds = append(adds, Entry{Ref: r, At: orNow(st.at, now), Value: st.value})
		}
	}
	for id, st := range pl.pushRemove {
		for _, r := range ix.refs(id) {
			removes = append(removes, Entry{Ref: r, RemoteRef: st.ref})
		}
	}
	if len(adds) > 0 || len(removes) > 0 {
		if err := p.Push(ctx, creds, access, f, adds, removes); err != nil {
			return nil, err
		}
	}
	var changed []int64
	err := s.store.Tx(ctx, func(q *db.Queries) error {
		for id, st := range pl.localSet {
			at := orNow(st.at, now)
			var err error
			switch f {
			case FeatHistory:
				err = q.SyncMarkPlayed(ctx, db.SyncMarkPlayedParams{UserID: conn.UserID, ItemID: id, At: nullUnix(at)})
			case FeatProgress:
				pos := int64(st.value / 100 * float64(ix.items[id].RuntimeMs))
				err = q.SyncSetProgress(ctx, db.SyncSetProgressParams{UserID: conn.UserID, ItemID: id, PositionMs: pos, At: nullUnix(at)})
			case FeatWatchlist:
				err = q.SyncAddWatchlist(ctx, db.SyncAddWatchlistParams{UserID: conn.UserID, ItemID: id, AddedAt: at.Unix()})
			case FeatRatings:
				err = q.SyncSetRating(ctx, db.SyncSetRatingParams{UserID: conn.UserID, ItemID: id,
					Rating: sql.NullFloat64{Float64: st.value, Valid: true}, RatedAt: nullUnix(at)})
			}
			if err != nil {
				return err
			}
			changed = append(changed, id)
		}
		for _, id := range pl.localRemove {
			var err error
			switch f {
			case FeatHistory:
				err = q.SyncMarkUnplayed(ctx, db.SyncMarkUnplayedParams{PlayedChangedAt: nullUnix(now), UserID: conn.UserID, ItemID: id})
			case FeatProgress:
				err = q.SyncSetProgress(ctx, db.SyncSetProgressParams{UserID: conn.UserID, ItemID: id, PositionMs: 0, At: nullUnix(now)})
			case FeatWatchlist:
				err = q.SyncRemoveWatchlist(ctx, db.SyncRemoveWatchlistParams{RemovedAt: nullUnix(now), UserID: conn.UserID, ItemID: id})
			case FeatRatings:
				err = q.SyncSetRating(ctx, db.SyncSetRatingParams{UserID: conn.UserID, ItemID: id, RatedAt: nullUnix(now)})
			}
			if err != nil {
				return err
			}
			changed = append(changed, id)
		}
		if err := q.ClearSyncStateFeature(ctx, db.ClearSyncStateFeatureParams{ConnectionID: conn.ID, Feature: string(f)}); err != nil {
			return err
		}
		for id, st := range pl.shadow {
			if err := q.UpsertSyncState(ctx, db.UpsertSyncStateParams{ConnectionID: conn.ID, Feature: string(f), ItemID: id,
				Value: st.value, RemoteAt: store.NullTime(st.at.Unix()), RemoteRef: st.ref}); err != nil {
				return err
			}
		}
		ins := func(kind string, item int64, winner, msg string) error {
			return q.InsertSyncEvent(ctx, db.InsertSyncEventParams{ConnectionID: conn.ID, Kind: kind, Feature: string(f),
				ItemID: store.NullTime(item), Winner: winner, Message: msg})
		}
		for i, c := range pl.conflicts {
			if i >= maxConflictLogs {
				break
			}
			msg := fmt.Sprintf("%s : « %s » modifié des deux côtés ; la version %s, plus récente, est conservée.",
				featureLabel[f], ix.items[c.itemID].Title, map[string]string{"local": "d'Aether", "remote": "de " + serviceName(p)}[c.winner])
			if err := ins("conflict", c.itemID, c.winner, msg); err != nil {
				return err
			}
		}
		if n := len(pl.localSet) + len(pl.localRemove); n > 0 {
			if err := ins("import", 0, "", fmt.Sprintf("%s : %d modification(s) reçue(s) de %s.", featureLabel[f], n, serviceName(p))); err != nil {
				return err
			}
		}
		if n := len(pl.pushAdd) + len(pl.pushRemove); n > 0 {
			if err := ins("export", 0, "", fmt.Sprintf("%s : %d modification(s) envoyée(s) à %s.", featureLabel[f], n, serviceName(p))); err != nil {
				return err
			}
		}
		return nil
	})
	return changed, err
}

func serviceName(p Provider) string {
	return map[string]string{"trakt": "Trakt", "simkl": "Simkl"}[p.Service()]
}

// ---- scrobbling ----

type scrobblePayload struct {
	Connection int64   `json:"connection"`
	Item       int64   `json:"item"`
	Action     Action  `json:"action"`
	Progress   float64 `json:"progress"`
	At         int64   `json:"at"`
}

func scrobblePrefix(connID, itemID int64) string { return fmt.Sprintf("c%d:i%d:", connID, itemID) }

// Scrobble queues a playback event for every link of the user with
// scrobbling on. It never blocks on the services.
func (s *Service) Scrobble(ctx context.Context, userID, itemID int64, a Action, progress float64) {
	conns, err := s.store.ListUserSyncConnections(ctx, userID)
	if err != nil {
		s.log.Error("scrobbling", "err", err)
		return
	}
	now := s.now()
	for _, c := range conns {
		p, err := s.provider(c.Service)
		if err != nil || c.Status != "connected" || c.Secret == nil || !s.enabled(p, c, FeatScrobble) {
			continue
		}
		key := fmt.Sprintf("%s%s:%d", scrobblePrefix(c.ID, itemID), a, now.UnixNano())
		pl := scrobblePayload{Connection: c.ID, Item: itemID, Action: a, Progress: progress, At: now.Unix()}
		if _, err := s.runner.Enqueue(ctx, JobScrobble, key, pl, 0); err != nil {
			s.log.Error("scrobbling", "connection", c.ID, "err", err)
		}
	}
}

// handleScrobble sends one playback event (job JobScrobble).
func (s *Service) handleScrobble(ctx context.Context, j jobs.Job) error {
	var pl scrobblePayload
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
	}
	conn, err := s.store.GetSyncConnectionByID(ctx, pl.Connection)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	p, err := s.provider(conn.Service)
	if err != nil || conn.Status != "connected" || !s.enabled(p, conn, FeatScrobble) {
		return nil
	}
	// A late "playing"/"paused" is meaningless; "stop" still counts.
	if pl.Action != ActStop && s.now().Unix()-pl.At > 10*60 {
		return nil
	}
	it, seriesIDs, err := s.store.SyncItemByID(ctx, pl.Item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var ref Ref
	switch it.Kind {
	case "movie":
		ref = Ref{Kind: "movie", IDs: parseIDs(it.ProviderIDs)}
	case "episode":
		ref = Ref{Kind: "episode", IDs: parseIDs(seriesIDs), Season: int(it.Season), Episode: int(it.Episode)}
		if it.Season < 0 || it.Episode < 0 {
			return nil
		}
	default:
		return nil
	}
	if ref.IDs.empty() || !p.Handles(FeatScrobble, ref.Kind) {
		return nil // nothing to identify the item with
	}
	err = s.withAccess(ctx, conn.ID, func(p Provider, c Creds, access string) error {
		return p.Scrobble(ctx, c, access, pl.Action, ref, pl.Progress)
	})
	if err != nil {
		return s.jobError(ctx, conn.ID, err)
	}
	if pl.Action == ActStop && pl.Progress >= 80 && s.enabled(p, conn, FeatHistory) {
		// The service recorded the play: the next merge must not send it again.
		err := s.store.UpsertSyncState(ctx, db.UpsertSyncStateParams{ConnectionID: conn.ID, Feature: string(FeatHistory),
			ItemID: it.ID, RemoteAt: store.NullTime(s.now().Unix())})
		if err != nil {
			return err
		}
	}
	return nil
}
