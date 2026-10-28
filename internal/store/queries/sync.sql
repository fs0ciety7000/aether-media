-- ---- application credentials ----

-- name: GetSyncApp :one
SELECT * FROM sync_apps WHERE service = ?;

-- name: UpsertSyncApp :exec
INSERT INTO sync_apps (service, secret) VALUES (?, ?)
ON CONFLICT (service) DO UPDATE SET secret = excluded.secret, updated_at = unixepoch();

-- name: DeleteSyncApp :exec
DELETE FROM sync_apps WHERE service = ?;

-- ---- connections ----

-- name: GetSyncConnection :one
SELECT * FROM sync_connections WHERE user_id = ? AND service = ?;

-- name: GetSyncConnectionByID :one
SELECT * FROM sync_connections WHERE id = ?;

-- name: ListUserSyncConnections :many
SELECT * FROM sync_connections WHERE user_id = ? ORDER BY service DESC;

-- name: ListActiveSyncConnections :many
SELECT * FROM sync_connections WHERE status = 'connected' ORDER BY id;

-- name: UpsertSyncPending :one
-- Starts (or restarts) a pairing; an existing link keeps working meanwhile.
INSERT INTO sync_connections (user_id, service, status, pending_secret, pending_user_code, pending_url,
                              pending_expires_at, pending_interval, pending_polled_at)
VALUES (sqlc.arg(user_id), sqlc.arg(service), 'pending', sqlc.arg(pending_secret), sqlc.arg(pending_user_code),
        sqlc.arg(pending_url), sqlc.arg(pending_expires_at), sqlc.arg(pending_interval), NULL)
ON CONFLICT (user_id, service) DO UPDATE SET
    pending_secret = excluded.pending_secret, pending_user_code = excluded.pending_user_code,
    pending_url = excluded.pending_url, pending_expires_at = excluded.pending_expires_at,
    pending_interval = excluded.pending_interval, pending_polled_at = NULL, updated_at = unixepoch()
RETURNING *;

-- name: SetSyncPendingPoll :exec
UPDATE sync_connections SET pending_interval = ?, pending_polled_at = ?, updated_at = unixepoch() WHERE id = ?;

-- name: ClearSyncPending :exec
UPDATE sync_connections SET pending_secret = NULL, pending_user_code = '', pending_url = '',
    pending_expires_at = NULL, pending_polled_at = NULL, updated_at = unixepoch()
WHERE id = ?;

-- name: SetSyncConnected :exec
UPDATE sync_connections SET status = 'connected', secret = ?, token_expires_at = ?, account = ?, remote_id = ?,
    connected_at = unixepoch(), pending_secret = NULL, pending_user_code = '', pending_url = '',
    pending_expires_at = NULL, pending_polled_at = NULL, last_error = '', last_error_at = NULL, updated_at = unixepoch()
WHERE id = ?;

-- name: SetSyncTokens :exec
UPDATE sync_connections SET secret = ?, token_expires_at = ?, updated_at = unixepoch() WHERE id = ?;

-- name: SetSyncReauth :exec
UPDATE sync_connections SET status = 'reauth_required', last_error = ?, last_error_at = unixepoch(), updated_at = unixepoch()
WHERE id = ?;

-- name: SetSyncFeatures :exec
UPDATE sync_connections SET features = ?, updated_at = unixepoch() WHERE id = ?;

-- name: SetSyncCursors :exec
UPDATE sync_connections SET cursors = ?, updated_at = unixepoch() WHERE id = ?;

-- name: SetSyncSuccess :exec
UPDATE sync_connections SET last_sync_at = unixepoch(), last_error = '', last_error_at = NULL, updated_at = unixepoch()
WHERE id = ?;

-- name: SetSyncError :exec
UPDATE sync_connections SET last_error = ?, last_error_at = unixepoch(), updated_at = unixepoch() WHERE id = ?;

-- name: DeleteSyncConnection :exec
DELETE FROM sync_connections WHERE id = ?;

-- ---- last synchronised state ----

-- name: ListSyncState :many
SELECT * FROM sync_state WHERE connection_id = ? AND feature = ?;

-- name: UpsertSyncState :exec
INSERT INTO sync_state (connection_id, feature, item_id, value, remote_at, remote_ref)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (connection_id, feature, item_id) DO UPDATE SET
    value = excluded.value, remote_at = excluded.remote_at, remote_ref = excluded.remote_ref;

-- name: DeleteSyncState :exec
DELETE FROM sync_state WHERE connection_id = ? AND feature = ? AND item_id = ?;

-- name: ClearSyncState :exec
DELETE FROM sync_state WHERE connection_id = ?;

-- ---- journal ----

-- name: InsertSyncEvent :exec
INSERT INTO sync_events (connection_id, kind, feature, item_id, winner, message) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListSyncEvents :many
SELECT e.id, e.kind, e.feature, e.item_id, e.winner, e.message, e.created_at, COALESCE(i.title, '') AS item_title
FROM sync_events e LEFT JOIN items i ON i.id = e.item_id
WHERE e.connection_id = ? ORDER BY e.id DESC LIMIT ?;

-- name: PurgeSyncEvents :execrows
DELETE FROM sync_events WHERE created_at < ?;

-- name: CountActiveJobs :one
SELECT count(*) FROM jobs WHERE kind = ? AND key = ? AND status IN ('queued', 'running');

-- ---- watchlist ----

-- name: AddToWatchlist :exec
INSERT INTO user_watchlist (user_id, item_id, added_at) VALUES (?, ?, unixepoch())
ON CONFLICT (user_id, item_id) DO UPDATE SET
    added_at = CASE WHEN user_watchlist.removed_at IS NULL THEN user_watchlist.added_at ELSE unixepoch() END,
    removed_at = NULL;

-- name: RemoveFromWatchlist :exec
UPDATE user_watchlist SET removed_at = unixepoch() WHERE user_id = ? AND item_id = ? AND removed_at IS NULL;

-- name: ListWatchlistIDs :many
SELECT item_id FROM user_watchlist WHERE user_id = ? AND removed_at IS NULL ORDER BY added_at DESC, item_id DESC;

-- name: ListUserWatchlist :many
SELECT * FROM user_watchlist WHERE user_id = ?;

-- name: SyncAddWatchlist :exec
INSERT INTO user_watchlist (user_id, item_id, added_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, item_id) DO UPDATE SET added_at = excluded.added_at, removed_at = NULL;

-- name: SyncRemoveWatchlist :exec
UPDATE user_watchlist SET removed_at = ? WHERE user_id = ? AND item_id = ?;

-- ---- user data written by imports ----

-- name: ListUserItemData :many
SELECT * FROM user_item_data WHERE user_id = ?;

-- name: SyncMarkPlayed :exec
INSERT INTO user_item_data (user_id, item_id, played, play_count, position_ms, last_played_at, played_changed_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(item_id), 1, 1, 0, sqlc.arg(at), sqlc.arg(at), unixepoch())
ON CONFLICT (user_id, item_id) DO UPDATE SET
    played = 1, play_count = max(user_item_data.play_count, 1), position_ms = 0,
    last_played_at = max(COALESCE(user_item_data.last_played_at, 0), excluded.last_played_at),
    played_changed_at = excluded.played_changed_at, updated_at = unixepoch();

-- name: SyncMarkUnplayed :exec
UPDATE user_item_data SET played = 0, position_ms = 0, played_changed_at = ?, updated_at = unixepoch()
WHERE user_id = ? AND item_id = ?;

-- name: SyncSetRating :exec
INSERT INTO user_item_data (user_id, item_id, rating, rated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id, item_id) DO UPDATE SET rating = excluded.rating, rated_at = excluded.rated_at, updated_at = unixepoch();

-- name: SyncSetProgress :exec
INSERT INTO user_item_data (user_id, item_id, position_ms, last_played_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(item_id), sqlc.arg(position_ms), sqlc.arg(at), unixepoch())
ON CONFLICT (user_id, item_id) DO UPDATE SET
    position_ms = excluded.position_ms,
    last_played_at = CASE WHEN excluded.position_ms > 0 THEN excluded.last_played_at ELSE user_item_data.last_played_at END,
    updated_at = unixepoch();

-- name: ClearSyncStateFeature :exec
DELETE FROM sync_state WHERE connection_id = ? AND feature = ?;

-- name: CountActiveJobsLike :one
SELECT count(*) FROM jobs WHERE kind = ? AND key LIKE ? AND status IN ('queued', 'running');
