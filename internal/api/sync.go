package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/syncsvc"
)

// requireSync answers 503 when synchronisation is not wired (tests, scan).
func (s *Server) requireSync(w http.ResponseWriter) bool {
	if s.Sync == nil {
		writeError(w, http.StatusServiceUnavailable, "sync_unavailable", "synchronisation indisponible")
		return false
	}
	return true
}

// syncError maps service errors onto HTTP answers. Messages never carry
// tokens: provider errors are built without request bodies.
func (s *Server) syncError(w http.ResponseWriter, err error) {
	var rl *syncsvc.RateLimitError
	var re *syncsvc.RemoteError
	switch {
	case errors.Is(err, syncsvc.ErrUnknownService):
		writeError(w, http.StatusNotFound, "unknown_service", "service inconnu (trakt ou simkl)")
	case errors.Is(err, syncsvc.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "sync_unavailable", "service non configuré par l'administrateur")
	case errors.Is(err, syncsvc.ErrManagedByConfig):
		writeError(w, http.StatusConflict, "managed_by_config", "identifiants fournis par la configuration du serveur")
	case errors.Is(err, syncsvc.ErrNotConnected):
		writeError(w, http.StatusNotFound, "not_connected", "aucune connexion à ce service")
	case errors.Is(err, syncsvc.ErrNotPending):
		writeError(w, http.StatusNotFound, "not_pending", "aucun appairage en cours")
	case errors.Is(err, syncsvc.ErrReauth):
		writeError(w, http.StatusConflict, "reauth_required", "reconnexion nécessaire")
	case errors.Is(err, syncsvc.ErrUnsupportedFeature):
		writeError(w, http.StatusBadRequest, "unsupported_feature", err.Error())
	case errors.Is(err, syncsvc.ErrInvalidCreds):
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.As(err, &rl):
		w.Header().Set("Retry-After", strconv.Itoa(int(max(rl.RetryAfter, time.Second)/time.Second)))
		writeError(w, http.StatusTooManyRequests, "rate_limited", rl.Error())
	case errors.As(err, &re), errors.Is(err, syncsvc.ErrUnauthorized), errors.Is(err, syncsvc.ErrNotFound):
		s.log.Warn("synchronisation", "err", err)
		writeError(w, http.StatusBadGateway, "remote_error", "le service ne répond pas comme prévu : "+err.Error())
	default:
		s.internalError(w, err)
	}
}

func uid(r *http.Request) int64 { return principalFrom(r).user.ID }

// syncList: GET /api/v1/me/sync
func (s *Server) syncList(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	out, err := s.Sync.Connections(r.Context(), uid(r))
	if err != nil {
		s.syncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// syncConnect: POST /api/v1/me/sync/{service}/connect
func (s *Server) syncConnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	out, err := s.Sync.Connect(r.Context(), uid(r), r.PathValue("service"))
	if err != nil {
		s.syncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// syncPoll: POST /api/v1/me/sync/{service}/poll
func (s *Server) syncPoll(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	out, err := s.Sync.Poll(r.Context(), uid(r), r.PathValue("service"))
	if err != nil {
		s.syncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// syncDisconnect: DELETE /api/v1/me/sync/{service}
func (s *Server) syncDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	if err := s.Sync.Disconnect(r.Context(), uid(r), r.PathValue("service")); err != nil {
		s.syncError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// syncPatch: PATCH /api/v1/me/sync/{service}
func (s *Server) syncPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	var req struct {
		Features map[syncsvc.Feature]bool `json:"features"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	out, err := s.Sync.SetFeatures(r.Context(), uid(r), r.PathValue("service"), req.Features)
	if err != nil {
		s.syncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// syncRun: POST /api/v1/me/sync/{service}/run
func (s *Server) syncRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	var req struct {
		Full bool `json:"full"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) // body optional
	queued, err := s.Sync.RunNow(r.Context(), uid(r), r.PathValue("service"), req.Full)
	if err != nil {
		s.syncError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": queued})
}

// syncEvents: GET /api/v1/me/sync/{service}/events?limit=50
func (s *Server) syncEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = min(v, 200)
	}
	out, err := s.Sync.Events(r.Context(), uid(r), r.PathValue("service"), limit)
	if err != nil {
		s.syncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// syncApps: GET /api/v1/sync/apps (admin)
func (s *Server) syncApps(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	out, err := s.Sync.Apps(r.Context())
	if err != nil {
		s.syncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// syncSetApp: PUT /api/v1/sync/apps/{service} (admin)
func (s *Server) syncSetApp(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	var req syncsvc.Creds
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	service := r.PathValue("service")
	if err := s.Sync.SetApp(r.Context(), service, req); err != nil {
		s.syncError(w, err)
		return
	}
	apps, err := s.Sync.Apps(r.Context())
	if err != nil {
		s.syncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apps[service])
}

// syncDeleteApp: DELETE /api/v1/sync/apps/{service} (admin)
func (s *Server) syncDeleteApp(w http.ResponseWriter, r *http.Request) {
	if !s.requireSync(w) {
		return
	}
	if err := s.Sync.DeleteApp(r.Context(), r.PathValue("service")); err != nil {
		s.syncError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// localChanged tells the synchronisation that the user's data moved.
func (s *Server) localChanged(r *http.Request, userID int64) {
	if s.Sync != nil {
		s.Sync.LocalChanged(r.Context(), userID)
	}
}

// ---- watchlist ----

// watchlist: GET /api/v1/watchlist
func (s *Server) watchlist(w http.ResponseWriter, r *http.Request) {
	ids, err := s.store.ListWatchlistIDs(r.Context(), uid(r))
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := []itemDTO{}
	if len(ids) > 0 {
		rows, _, err := s.browse(r, store.BrowseParams{IDs: ids, Limit: len(ids)})
		if err != nil {
			s.internalError(w, err)
			return
		}
		pos := map[int64]int{}
		for i, id := range ids {
			pos[id] = i
		}
		slices.SortFunc(rows, func(a, b store.BrowseRow) int { return pos[a.ID] - pos[b.ID] })
		if out, err = s.itemDTOs(r, rows); err != nil {
			s.internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) watchlistItem(w http.ResponseWriter, r *http.Request) (db.Item, bool) {
	it, ok := s.loadItem(w, r)
	if !ok {
		return it, false
	}
	if !s.visible(r, it.ID) {
		writeError(w, http.StatusNotFound, "not_found", "élément introuvable")
		return it, false
	}
	if it.Kind != "movie" && it.Kind != "series" {
		writeError(w, http.StatusBadRequest, "invalid_kind", "seuls les films et les séries vont dans la watchlist")
		return it, false
	}
	return it, true
}

// watchlistAdd: PUT /api/v1/watchlist/{id}
func (s *Server) watchlistAdd(w http.ResponseWriter, r *http.Request) {
	it, ok := s.watchlistItem(w, r)
	if !ok {
		return
	}
	if err := s.store.AddToWatchlist(r.Context(), db.AddToWatchlistParams{UserID: uid(r), ItemID: it.ID}); err != nil {
		s.internalError(w, err)
		return
	}
	s.userDataChanged(r, it.ID)
	w.WriteHeader(http.StatusNoContent)
}

// watchlistRemove: DELETE /api/v1/watchlist/{id}
func (s *Server) watchlistRemove(w http.ResponseWriter, r *http.Request) {
	it, ok := s.watchlistItem(w, r)
	if !ok {
		return
	}
	if err := s.store.RemoveFromWatchlist(r.Context(), db.RemoveFromWatchlistParams{UserID: uid(r), ItemID: it.ID}); err != nil {
		s.internalError(w, err)
		return
	}
	s.userDataChanged(r, it.ID)
	w.WriteHeader(http.StatusNoContent)
}

// userDataChanged announces a change of the user's data on an item and
// schedules its synchronisation.
func (s *Server) userDataChanged(r *http.Request, itemID int64) {
	if s.Hub != nil {
		s.Hub.PublishUser(uid(r), "userdata.changed", map[string]any{"userId": uid(r), "itemId": itemID})
	}
	s.localChanged(r, uid(r))
}
