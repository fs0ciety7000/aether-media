package store

import (
	"context"
	"database/sql"
)

// SyncItem is a movie, series or episode the viewer can see, with what
// external services need to identify it.
type SyncItem struct {
	ID          int64
	Kind        string // movie | series | episode
	Title       string
	ProviderIDs string // JSON object
	RuntimeMs   int64
	// Episodes only.
	SeriesID   int64
	Season     int64
	Episode    int64
	EpisodeEnd int64 // last episode of a multi-episode file (0 = single)
}

// SyncItems lists the items the viewer may see that synchronisation can
// match: movies, series and their episodes (season number from the parent).
func (s *Store) SyncItems(ctx context.Context, v Viewer) ([]SyncItem, error) {
	vis, args := v.Visibility()
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.kind, i.title, i.provider_ids, COALESCE(i.runtime_ms, 0),
		COALESCE(se.parent_id, CASE WHEN i.kind = 'episode' THEN i.parent_id END, 0), COALESCE(se.index_number, i.parent_index_number, -1),
		COALESCE(i.index_number, -1), COALESCE(i.index_number_end, 0)
		FROM items i LEFT JOIN items se ON i.kind = 'episode' AND se.id = i.parent_id AND se.kind = 'season'
		WHERE i.missing = 0 AND i.kind IN ('movie', 'series', 'episode') AND `+vis, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncItem
	for rows.Next() {
		var it SyncItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.Title, &it.ProviderIDs, &it.RuntimeMs,
			&it.SeriesID, &it.Season, &it.Episode, &it.EpisodeEnd); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// NullTime is a helper for optional unix timestamps.
func NullTime(t int64) sql.NullInt64 { return sql.NullInt64{Int64: t, Valid: t > 0} }

// ViewerFor builds the listing restrictions of a user (as for their API
// requests), for background work done on their behalf.
func (s *Store) ViewerFor(ctx context.Context, userID int64) (Viewer, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return Viewer{}, err
	}
	v := Viewer{UserID: u.ID, BlockUnrated: u.BlockUnrated == 1}
	if u.MaxRatingAge.Valid {
		v.MaxAge, v.HasMaxAge = int(u.MaxRatingAge.Int64), true
	}
	if u.AllLibraries == 0 && u.IsAdmin == 0 {
		libs, err := s.ListUserLibraries(ctx, u.ID)
		if err != nil {
			return Viewer{}, err
		}
		v.LibraryIDs = libs
		if v.LibraryIDs == nil {
			v.LibraryIDs = []int64{}
		}
	}
	return v, nil
}

// SyncItemByID returns one item as SyncItems would (no visibility check),
// with the provider ids of its series for an episode.
func (s *Store) SyncItemByID(ctx context.Context, id int64) (SyncItem, string, error) {
	var it SyncItem
	var seriesIDs string
	err := s.DB.QueryRowContext(ctx, `SELECT i.id, i.kind, i.title, i.provider_ids, COALESCE(i.runtime_ms, 0),
		COALESCE(se.parent_id, CASE WHEN i.kind = 'episode' THEN i.parent_id END, 0),
		COALESCE(se.index_number, i.parent_index_number, -1),
		COALESCE(i.index_number, -1), COALESCE(i.index_number_end, 0),
		COALESCE((SELECT g.provider_ids FROM items g WHERE g.id = COALESCE(se.parent_id, i.parent_id) AND g.kind = 'series'), '{}')
		FROM items i LEFT JOIN items se ON i.kind = 'episode' AND se.id = i.parent_id AND se.kind = 'season'
		WHERE i.id = ?`, id).Scan(&it.ID, &it.Kind, &it.Title, &it.ProviderIDs, &it.RuntimeMs,
		&it.SeriesID, &it.Season, &it.Episode, &it.EpisodeEnd, &seriesIDs)
	return it, seriesIDs, err
}
