-- name: UpsertItem :one
-- Insert or refresh an item identified by its stable key. A rediscovered item
-- loses its missing flag. Metadata fields owned by providers (overview,
-- original_title...) are not touched here.
INSERT INTO items (
    key, library_id, parent_id, kind, title, sort_title, year, path,
    index_number, index_number_end, parent_index_number, edition, air_date,
    provider_ids
) VALUES (
    sqlc.arg(key), sqlc.arg(library_id), sqlc.narg(parent_id), sqlc.arg(kind),
    sqlc.arg(title), sqlc.arg(sort_title), sqlc.narg(year), sqlc.narg(path),
    sqlc.narg(index_number), sqlc.narg(index_number_end), sqlc.narg(parent_index_number),
    sqlc.narg(edition), sqlc.narg(air_date), sqlc.arg(provider_ids)
)
ON CONFLICT (key) DO UPDATE SET
    parent_id           = excluded.parent_id,
    title               = CASE WHEN items.locked_fields LIKE '%"title"%' THEN items.title ELSE excluded.title END,
    sort_title          = CASE WHEN items.locked_fields LIKE '%"title"%' THEN items.sort_title ELSE excluded.sort_title END,
    year                = COALESCE(excluded.year, items.year),
    path                = excluded.path,
    index_number        = excluded.index_number,
    index_number_end    = excluded.index_number_end,
    parent_index_number = excluded.parent_index_number,
    edition             = excluded.edition,
    air_date            = excluded.air_date,
    provider_ids        = CASE WHEN excluded.provider_ids = '{}' THEN items.provider_ids ELSE excluded.provider_ids END,
    missing             = 0,
    date_updated        = unixepoch()
RETURNING id;

-- name: GetItem :one
SELECT * FROM items WHERE id = ?;

-- name: ListItems :many
SELECT * FROM items
WHERE library_id = COALESCE(sqlc.narg(library_id), library_id)
  AND kind = COALESCE(sqlc.narg(kind), kind)
  AND IFNULL(parent_id, 0) = COALESCE(CAST(sqlc.narg(parent_id) AS INTEGER), IFNULL(parent_id, 0))
  AND missing = 0
  AND identify_status = COALESCE(sqlc.narg(identify_status), identify_status)
  AND (sort_title > sqlc.arg(after_sort_title)
       OR (sort_title = sqlc.arg(after_sort_title) AND id > CAST(sqlc.arg(after_id) AS INTEGER)))
ORDER BY sort_title, id
LIMIT sqlc.arg(lim);

-- name: CountChildren :one
SELECT count(*) FROM items WHERE parent_id = ? AND missing = 0;

-- name: MarkOrphansMissing :execrows
-- Leaf items (movie, episode, track) that no longer have any media file.
UPDATE items SET missing = 1, date_updated = unixepoch()
WHERE items.library_id = sqlc.arg(library_id) AND items.missing = 0
  AND items.kind IN ('movie', 'episode', 'track')
  AND NOT EXISTS (SELECT 1 FROM media_files mf WHERE mf.item_id = items.id);

-- name: MarkEmptyContainersMissing :execrows
-- Containers (season, series, album, artist) whose children are all missing.
UPDATE items SET missing = 1, date_updated = unixepoch()
WHERE items.library_id = sqlc.arg(library_id) AND items.missing = 0
  AND items.kind IN ('season', 'series', 'album', 'artist')
  AND NOT EXISTS (SELECT 1 FROM items c WHERE c.parent_id = items.id AND c.missing = 0);

-- name: PurgeMissingItems :execrows
DELETE FROM items WHERE missing = 1 AND date_updated < sqlc.arg(before);

-- name: UpdateTrackFromTags :exec
UPDATE items SET
    title        = CASE WHEN locked_fields LIKE '%"title"%' THEN title ELSE sqlc.arg(title) END,
    sort_title   = CASE WHEN locked_fields LIKE '%"title"%' THEN sort_title ELSE sqlc.arg(sort_title) END,
    index_number = COALESCE(sqlc.narg(index_number), index_number),
    parent_index_number = COALESCE(sqlc.narg(parent_index_number), parent_index_number),
    year         = COALESCE(sqlc.narg(year), year),
    date_updated = unixepoch()
WHERE id = sqlc.arg(id);

-- name: SetItemRuntime :exec
UPDATE items SET runtime_ms = ? WHERE id = ?;

-- name: LibraryStats :many
SELECT kind, count(*) AS n FROM items WHERE library_id = ? AND missing = 0 GROUP BY kind;
