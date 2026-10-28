-- name: ListSettings :many
SELECT * FROM settings ORDER BY key;

-- name: GetSetting :one
SELECT * FROM settings WHERE key = ?;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value, secret) VALUES (?, ?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value, secret = excluded.secret, updated_at = unixepoch();

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = ?;
