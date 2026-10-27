-- name: ListLibraries :many
SELECT * FROM libraries ORDER BY name;

-- name: CreateLibrary :one
INSERT INTO libraries (name, kind) VALUES (?, ?) RETURNING *;

-- name: AddLibraryPath :exec
INSERT INTO library_paths (library_id, path) VALUES (?, ?);

-- name: ListLibraryPaths :many
SELECT * FROM library_paths WHERE library_id = ? ORDER BY path;
