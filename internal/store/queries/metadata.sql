-- name: ApplyItemMetadata :exec
-- Fields are only written when not locked; callers pass the merged values.
UPDATE items SET
    title           = CASE WHEN locked_fields LIKE '%"title"%' THEN title ELSE COALESCE(sqlc.narg(title), title) END,
    sort_title      = CASE WHEN locked_fields LIKE '%"title"%' THEN sort_title ELSE COALESCE(sqlc.narg(sort_title), sort_title) END,
    original_title  = CASE WHEN locked_fields LIKE '%"originalTitle"%' THEN original_title ELSE COALESCE(sqlc.narg(original_title), original_title) END,
    overview        = CASE WHEN locked_fields LIKE '%"overview"%' THEN overview ELSE COALESCE(sqlc.narg(overview), overview) END,
    tagline         = CASE WHEN locked_fields LIKE '%"tagline"%' THEN tagline ELSE COALESCE(sqlc.narg(tagline), tagline) END,
    year            = CASE WHEN locked_fields LIKE '%"year"%' THEN year ELSE COALESCE(sqlc.narg(year), year) END,
    premiere_date   = CASE WHEN locked_fields LIKE '%"premiereDate"%' THEN premiere_date ELSE COALESCE(sqlc.narg(premiere_date), premiere_date) END,
    end_date        = COALESCE(sqlc.narg(end_date), end_date),
    status          = COALESCE(sqlc.narg(status), status),
    runtime_ms      = COALESCE(runtime_ms, sqlc.narg(runtime_ms)),
    official_rating = CASE WHEN locked_fields LIKE '%"officialRating"%' THEN official_rating ELSE COALESCE(sqlc.narg(official_rating), official_rating) END,
    community_rating = COALESCE(sqlc.narg(community_rating), community_rating),
    collection      = COALESCE(sqlc.narg(collection), collection),
    provider_ids    = sqlc.arg(provider_ids),
    identify_status = sqlc.arg(identify_status),
    metadata_refreshed_at = unixepoch(),
    date_updated    = unixepoch()
WHERE id = sqlc.arg(id);

-- name: SetIdentify :exec
UPDATE items SET provider_ids = ?, identify_status = ?, date_updated = unixepoch() WHERE id = ?;

-- name: UpdateItemFields :exec
-- Manual edit: every passed field is written and locked by the caller.
UPDATE items SET
    title          = COALESCE(sqlc.narg(title), title),
    sort_title     = COALESCE(sqlc.narg(sort_title), sort_title),
    original_title = COALESCE(sqlc.narg(original_title), original_title),
    overview       = COALESCE(sqlc.narg(overview), overview),
    tagline        = COALESCE(sqlc.narg(tagline), tagline),
    year           = COALESCE(sqlc.narg(year), year),
    official_rating = COALESCE(sqlc.narg(official_rating), official_rating),
    locked_fields  = sqlc.arg(locked_fields),
    date_updated   = unixepoch()
WHERE id = sqlc.arg(id);

-- name: ListItemsNeedingRefresh :many
-- Top-level items never refreshed, plus seasons holding unrefreshed episodes.
SELECT i.id, i.kind FROM items i
WHERE i.library_id = sqlc.arg(library_id) AND i.missing = 0 AND i.metadata_refreshed_at IS NULL
  AND i.kind IN ('movie', 'series', 'artist', 'album')
UNION
SELECT DISTINCT s.id, s.kind FROM items e JOIN items s ON s.id = e.parent_id
WHERE e.library_id = sqlc.arg(library_id) AND e.missing = 0 AND e.metadata_refreshed_at IS NULL
  AND e.kind = 'episode';

-- name: ListChildItems :many
SELECT * FROM items WHERE parent_id = ? AND missing = 0 ORDER BY sort_title, id;

-- name: ClearItemGenres :exec
DELETE FROM item_genres WHERE item_id = ?;

-- name: UpsertGenre :one
INSERT INTO genres (name) VALUES (?) ON CONFLICT (name) DO UPDATE SET name = genres.name RETURNING id;

-- name: AddItemGenre :exec
INSERT OR IGNORE INTO item_genres (item_id, genre_id) VALUES (?, ?);

-- name: ClearItemStudios :exec
DELETE FROM item_studios WHERE item_id = ?;

-- name: UpsertStudio :one
INSERT INTO studios (name) VALUES (?) ON CONFLICT (name) DO UPDATE SET name = studios.name RETURNING id;

-- name: AddItemStudio :exec
INSERT OR IGNORE INTO item_studios (item_id, studio_id) VALUES (?, ?);

-- name: ClearItemPeople :exec
DELETE FROM item_people WHERE item_id = ?;

-- name: UpsertPerson :one
INSERT INTO people (name, key, provider_ids, image_url) VALUES (?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET
    name = excluded.name,
    provider_ids = CASE WHEN excluded.provider_ids = '{}' THEN people.provider_ids ELSE excluded.provider_ids END,
    image_url = CASE WHEN excluded.image_url = '' THEN people.image_url ELSE excluded.image_url END,
    image_hash = CASE WHEN excluded.image_url = '' OR excluded.image_url = people.image_url THEN people.image_hash ELSE '' END
RETURNING id;

-- name: GetPersonImage :one
SELECT id, image_url, image_hash FROM people WHERE id = ?;

-- name: SetPersonImageHash :exec
-- Only if the source did not change while it was being fetched.
UPDATE people SET image_hash = ? WHERE id = ? AND image_url = ?;

-- name: AddItemPerson :exec
INSERT OR IGNORE INTO item_people (item_id, person_id, kind, role, ord) VALUES (?, ?, ?, ?, ?);

-- name: ListItemGenres :many
SELECT g.name FROM genres g JOIN item_genres ig ON ig.genre_id = g.id WHERE ig.item_id = ? ORDER BY g.name;

-- name: ListItemStudios :many
SELECT s.name FROM studios s JOIN item_studios ist ON ist.studio_id = s.id WHERE ist.item_id = ? ORDER BY s.name;

-- name: ListItemPeople :many
SELECT p.id, p.name, p.image_url, ip.kind, ip.role, ip.ord
FROM people p JOIN item_people ip ON ip.person_id = p.id
WHERE ip.item_id = ? ORDER BY CASE ip.kind WHEN 'director' THEN 0 WHEN 'actor' THEN 1 ELSE 2 END, ip.ord;

-- name: UpsertImage :one
-- A changed source resets the cached analysis so the image is fetched again.
INSERT INTO images (item_id, type, idx, source, is_local, provider, language)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (item_id, type, idx) DO UPDATE SET
    hash        = CASE WHEN images.source = excluded.source THEN images.hash ELSE NULL END,
    fetch_error = CASE WHEN images.source = excluded.source THEN images.fetch_error ELSE '' END,
    source      = excluded.source,
    is_local    = excluded.is_local,
    provider    = excluded.provider,
    language    = excluded.language
RETURNING id, hash;

-- name: DeleteImagesFrom :exec
-- Drops extra images beyond the new count for a type.
DELETE FROM images WHERE item_id = ? AND type = ? AND idx >= ?;

-- name: GetImage :one
SELECT * FROM images WHERE id = ?;

-- name: GetItemImage :one
SELECT * FROM images WHERE item_id = ? AND type = ? AND idx = ?;

-- name: ListItemImages :many
SELECT * FROM images WHERE item_id = ? ORDER BY type, idx;

-- name: SetImageFetched :exec
UPDATE images SET hash = ?, width = ?, height = ?, blurhash = ?, dominant = ?, fetch_error = '' WHERE id = ?;

-- name: SetImageError :exec
UPDATE images SET fetch_error = ? WHERE id = ?;

-- name: ListRefreshRoots :many
-- Every top-level item of a library (a refresh cascades to its children).
SELECT i.id FROM items i
WHERE i.library_id = ? AND i.missing = 0 AND i.parent_id IS NULL
  AND i.kind IN ('movie', 'series', 'artist', 'album')
ORDER BY i.id;
