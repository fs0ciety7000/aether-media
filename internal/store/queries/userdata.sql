-- name: GetUserData :one
SELECT * FROM user_item_data WHERE user_id = ? AND item_id = ?;

-- name: SaveProgress :exec
INSERT INTO user_item_data (user_id, item_id, position_ms, last_played_at, updated_at)
VALUES (?, ?, ?, unixepoch(), unixepoch())
ON CONFLICT (user_id, item_id) DO UPDATE SET
    position_ms = excluded.position_ms, last_played_at = unixepoch(), updated_at = unixepoch();

-- name: MarkPlayed :exec
-- A finished viewing: counts one more play.
INSERT INTO user_item_data (user_id, item_id, played, play_count, position_ms, last_played_at, updated_at)
VALUES (?, ?, 1, 1, 0, unixepoch(), unixepoch())
ON CONFLICT (user_id, item_id) DO UPDATE SET
    played = 1, play_count = user_item_data.play_count + 1, position_ms = 0,
    last_played_at = unixepoch(), updated_at = unixepoch();

-- name: MarkWatched :exec
-- "Mark as watched" by hand: no extra play counted.
INSERT INTO user_item_data (user_id, item_id, played, play_count, position_ms, last_played_at, updated_at)
VALUES (?, ?, 1, 1, 0, unixepoch(), unixepoch())
ON CONFLICT (user_id, item_id) DO UPDATE SET
    played = 1, play_count = max(user_item_data.play_count, 1), position_ms = 0,
    last_played_at = COALESCE(user_item_data.last_played_at, unixepoch()), updated_at = unixepoch();

-- name: MarkUnplayed :exec
UPDATE user_item_data SET played = 0, position_ms = 0, updated_at = unixepoch() WHERE user_id = ? AND item_id = ?;

-- name: SetFavorite :exec
INSERT INTO user_item_data (user_id, item_id, is_favorite) VALUES (?, ?, ?)
ON CONFLICT (user_id, item_id) DO UPDATE SET is_favorite = excluded.is_favorite, updated_at = unixepoch();

-- name: SetRating :exec
INSERT INTO user_item_data (user_id, item_id, rating) VALUES (?, ?, ?)
ON CONFLICT (user_id, item_id) DO UPDATE SET rating = excluded.rating, updated_at = unixepoch();

-- name: SetPosition :exec
INSERT INTO user_item_data (user_id, item_id, position_ms) VALUES (?, ?, ?)
ON CONFLICT (user_id, item_id) DO UPDATE SET position_ms = excluded.position_ms, updated_at = unixepoch();

-- name: ListPlayableDescendants :many
-- Episodes of a series/season (or tracks of an album/artist), any depth up to 3.
SELECT i.id FROM items i WHERE i.missing = 0 AND i.kind IN ('episode', 'track', 'movie') AND (
    i.parent_id = sqlc.arg(id)
    OR i.parent_id IN (SELECT c.id FROM items c WHERE c.parent_id = sqlc.arg(id))
    OR i.id = sqlc.arg(id));

-- name: SetItemRatingAge :exec
UPDATE items SET rating_age = ? WHERE id = ?;

-- name: ListItemsForRatingAge :many
SELECT id, official_rating FROM items WHERE official_rating IS NOT NULL AND official_rating != '';
