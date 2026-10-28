-- name: ListLibraries :many
SELECT * FROM libraries ORDER BY name;

-- name: CreateLibrary :one
INSERT INTO libraries (name, kind) VALUES (?, ?) RETURNING *;


-- name: GetLibrary :one
SELECT * FROM libraries WHERE id = ?;

-- name: DeleteLibrary :execrows
DELETE FROM libraries WHERE id = ?;


-- name: CreateSource :one
INSERT INTO library_sources (library_id, kind, name, root, config, secret)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateSource :one
UPDATE library_sources SET name = ?, root = ?, config = ?, secret = ?, status = 'unknown', last_error = ''
WHERE id = ? AND library_id = ? RETURNING *;

-- name: GetSource :one
SELECT * FROM library_sources WHERE id = ?;

-- name: ListSources :many
SELECT * FROM library_sources WHERE library_id = ? ORDER BY id;

-- name: ListAllSources :many
SELECT * FROM library_sources ORDER BY library_id, id;

-- name: DeleteSource :execrows
DELETE FROM library_sources WHERE id = ? AND library_id = ?;

-- name: SetSourceStatus :exec
UPDATE library_sources SET status = ?, last_error = ?,
    last_seen = CASE WHEN ? = 'online' THEN unixepoch() ELSE last_seen END
WHERE id = ?;
