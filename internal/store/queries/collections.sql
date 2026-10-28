-- name: CreateCollection :one
INSERT INTO collections (owner_id, name, shared) VALUES (?, ?, ?) RETURNING *;

-- name: GetCollection :one
SELECT * FROM collections WHERE id = ?;

-- name: ListCollectionsForUser :many
SELECT c.*, (SELECT count(*) FROM collection_items ci JOIN items i ON i.id = ci.item_id WHERE ci.collection_id = c.id AND i.missing = 0) AS n
FROM collections c WHERE c.owner_id = ? OR c.shared = 1 ORDER BY c.name COLLATE NOCASE;

-- name: UpdateCollection :exec
UPDATE collections SET name = ?, shared = ? WHERE id = ?;

-- name: DeleteCollection :exec
DELETE FROM collections WHERE id = ?;

-- name: AddCollectionItem :exec
INSERT OR IGNORE INTO collection_items (collection_id, item_id, ord)
VALUES (sqlc.arg(collection_id), sqlc.arg(item_id), COALESCE((SELECT max(ord) + 1 FROM collection_items WHERE collection_id = sqlc.arg(collection_id)), 0));

-- name: RemoveCollectionItem :exec
DELETE FROM collection_items WHERE collection_id = ? AND item_id = ?;

-- name: ListCollectionItemIDs :many
SELECT item_id FROM collection_items WHERE collection_id = ? ORDER BY ord, added_at;

-- name: ListAutoCollections :many
SELECT collection, count(*) AS n FROM items
WHERE collection IS NOT NULL AND collection != '' AND missing = 0 AND kind IN ('movie', 'series')
GROUP BY collection HAVING count(*) >= 2 ORDER BY collection COLLATE NOCASE;

-- name: ListAutoCollectionItemIDs :many
SELECT id FROM items WHERE collection = ? AND missing = 0 AND kind IN ('movie', 'series') ORDER BY premiere_date, year, sort_title;
