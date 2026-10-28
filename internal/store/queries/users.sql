-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: CountAdmins :one
SELECT count(*) FROM users WHERE is_admin = 1;

-- name: CreateUser :one
INSERT INTO users (name, password_hash, is_admin, all_libraries, max_rating_age, block_unrated)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByName :one
SELECT * FROM users WHERE name = ? COLLATE NOCASE;

-- name: ListUsers :many
SELECT * FROM users ORDER BY name COLLATE NOCASE;

-- name: UpdateUser :exec
UPDATE users SET name = ?, is_admin = ?, all_libraries = ?, max_rating_age = ?, block_unrated = ? WHERE id = ?;

-- name: SetUserPassword :exec
UPDATE users SET password_hash = ? WHERE id = ?;

-- name: SetUserPrefs :exec
UPDATE users SET prefs = ? WHERE id = ?;

-- name: TouchUser :exec
UPDATE users SET last_seen = unixepoch() WHERE id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: ListUserLibraries :many
SELECT library_id FROM user_library_access WHERE user_id = ? ORDER BY library_id;

-- name: ClearUserLibraries :exec
DELETE FROM user_library_access WHERE user_id = ?;

-- name: AddUserLibrary :exec
INSERT OR IGNORE INTO user_library_access (user_id, library_id) VALUES (?, ?);

-- name: CreateSession :exec
INSERT INTO sessions (id, token_hash, user_id, device_name, client) VALUES (?, ?, ?, ?, ?);

-- name: GetSessionByToken :one
SELECT * FROM sessions WHERE token_hash = ?;

-- name: TouchSession :exec
UPDATE sessions SET last_seen = unixepoch() WHERE token_hash = ?;

-- name: ListUserSessions :many
SELECT * FROM sessions WHERE user_id = ? ORDER BY last_seen DESC;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: DeleteUserSession :execrows
DELETE FROM sessions WHERE user_id = ? AND id = ?;

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE user_id = ? AND token_hash != ?;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = ?;

-- name: PurgeSessions :exec
DELETE FROM sessions WHERE last_seen < ?;
