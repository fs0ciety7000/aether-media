package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/syncsvc"
)

// play is a playback session as seen by progress tracking.
type play struct {
	userID, itemID int64
	kind           string
	durationMs     int64
	positionMs     int64
	finished       bool // already counted as played
	paused         bool // last state reported to scrobbling
	updated        time.Time
}

// playTracker maps stream sessions to users so pings record progress.
type playTracker struct {
	mu    sync.Mutex
	plays map[string]*play
}

func newPlayTracker() *playTracker { return &playTracker{plays: map[string]*play{}} }

func (t *playTracker) start(sid string, p *play) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, old := range t.plays { // forget sessions abandoned long ago
		if time.Since(old.updated) > 6*time.Hour {
			delete(t.plays, id)
		}
	}
	p.updated = time.Now()
	t.plays[sid] = p
}

// get returns the play of sid if it belongs to userID.
func (t *playTracker) get(sid string, userID int64) (*play, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.plays[sid]
	if !ok || p.userID != userID {
		return nil, false
	}
	return p, true
}

func (t *playTracker) end(sid string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.plays, sid)
}

// watchedEnough decides when an item counts as played: 90 % of its
// duration, or close enough to the end (credits).
func watchedEnough(kind string, pos, dur int64) bool {
	if dur <= 0 || pos <= 0 {
		return false
	}
	tail := int64(2 * time.Minute / time.Millisecond)
	if kind != "movie" {
		tail = int64(30 * time.Second / time.Millisecond)
	}
	return pos*10 >= dur*9 || (dur > 4*tail && dur-pos <= tail)
}

// percent is a playback position as a percentage of the duration.
func percent(pos, dur int64) float64 {
	if dur <= 0 || pos <= 0 {
		return 0
	}
	return min(100, float64(pos)*100/float64(dur))
}

// scrobble forwards a playback event to Trakt / Simkl (queued, never blocking).
func (s *Server) scrobble(r *http.Request, p *play, a syncsvc.Action, progress float64) {
	if s.Sync != nil {
		s.Sync.Scrobble(r.Context(), p.userID, p.itemID, a, progress)
	}
}

type pingRequest struct {
	PositionMs int64 `json:"positionMs"`
	Paused     bool  `json:"paused"`
}

// record saves the position of a play, or marks the item played once
// enough was watched.
func (s *Server) record(r *http.Request, p *play, pos int64) {
	ctx := r.Context()
	p.positionMs, p.updated = pos, time.Now()
	if p.finished {
		return
	}
	if watchedEnough(p.kind, pos, p.durationMs) {
		p.finished = true
		if err := s.store.MarkPlayed(ctx, db.MarkPlayedParams{UserID: p.userID, ItemID: p.itemID}); err != nil {
			s.log.Error("progression", "err", err)
		}
		if s.Hub != nil {
			s.Hub.Publish("userdata.changed", map[string]any{"userId": p.userID, "itemId": p.itemID})
		}
		// The service records the play itself (Trakt counts a stop ≥ 80 % as watched).
		s.scrobble(r, p, syncsvc.ActStop, max(percent(pos, p.durationMs), 90))
		s.localChanged(r, p.userID)
		return
	}
	if pos > 0 {
		if err := s.store.SaveProgress(ctx, db.SaveProgressParams{UserID: p.userID, ItemID: p.itemID, PositionMs: pos}); err != nil {
			s.log.Error("progression", "err", err)
		}
	}
}

type userDataRequest struct {
	Played     *bool       `json:"played"`
	Favorite   *bool       `json:"favorite"`
	Rating     optionalNum `json:"rating"` // null clears the rating
	PositionMs *int64      `json:"positionMs"`
}

// optionalNum tells an absent field from an explicit null.
type optionalNum struct {
	Set   bool
	Value *float64
}

func (o *optionalNum) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// putUserData: PUT /api/v1/items/{id}/userdata
func (s *Server) putUserData(w http.ResponseWriter, r *http.Request) {
	it, ok := s.loadItem(w, r)
	if !ok {
		return
	}
	if !s.visible(r, it.ID) {
		writeError(w, http.StatusNotFound, "not_found", "élément introuvable")
		return
	}
	var req userDataRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	if req.Rating.Value != nil && (*req.Rating.Value < 0 || *req.Rating.Value > 10) {
		writeError(w, http.StatusBadRequest, "invalid_rating", "note entre 0 et 10")
		return
	}
	uid := principalFrom(r).user.ID
	ctx := r.Context()
	err := s.store.Tx(ctx, func(q *db.Queries) error {
		if req.Played != nil {
			// Series, seasons, albums: apply to every playable descendant.
			ids, err := q.ListPlayableDescendants(ctx, sql.NullInt64{Int64: it.ID, Valid: true})
			if err != nil {
				return err
			}
			for _, id := range ids {
				if *req.Played {
					err = q.MarkWatched(ctx, db.MarkWatchedParams{UserID: uid, ItemID: id})
				} else {
					err = q.MarkUnplayed(ctx, db.MarkUnplayedParams{UserID: uid, ItemID: id})
				}
				if err != nil {
					return err
				}
			}
		}
		if req.Favorite != nil {
			if err := q.SetFavorite(ctx, db.SetFavoriteParams{UserID: uid, ItemID: it.ID, IsFavorite: b2i(*req.Favorite)}); err != nil {
				return err
			}
		}
		if req.Rating.Set {
			rating := sql.NullFloat64{}
			if req.Rating.Value != nil {
				rating = sql.NullFloat64{Float64: *req.Rating.Value, Valid: true}
			}
			if err := q.SetRating(ctx, db.SetRatingParams{UserID: uid, ItemID: it.ID, Rating: rating}); err != nil {
				return err
			}
		}
		if req.PositionMs != nil && *req.PositionMs >= 0 {
			return q.SetPosition(ctx, db.SetPositionParams{UserID: uid, ItemID: it.ID, PositionMs: *req.PositionMs})
		}
		return nil
	})
	if err != nil {
		s.internalError(w, err)
		return
	}
	if req.Played != nil || req.Rating.Set || req.PositionMs != nil {
		s.localChanged(r, uid)
	}
	rows, _, err := s.browse(r, storeIDs(it.ID))
	if err != nil || len(rows) == 0 {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userDataOf(rows[0]))
}
