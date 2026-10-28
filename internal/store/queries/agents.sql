-- name: UpsertAgent :one
INSERT INTO remote_agents (uid, name, url, secret, version, fingerprint, status, last_seen)
VALUES (?, ?, ?, ?, ?, ?, 'online', unixepoch())
ON CONFLICT (uid) DO UPDATE SET
    name = excluded.name, url = excluded.url, secret = excluded.secret, version = excluded.version,
    fingerprint = excluded.fingerprint, status = 'online', last_error = '', last_seen = unixepoch()
RETURNING *;

-- name: RepairAgent :one
-- Pairs an existing row again (new token, address, certificate; the uid
-- of a legacy row becomes the agent's real one).
UPDATE remote_agents SET uid = ?, name = ?, url = ?, secret = ?, version = ?, fingerprint = ?,
    status = 'online', last_error = '', last_seen = unixepoch()
WHERE id = ?
RETURNING *;

-- name: GetAgent :one
SELECT * FROM remote_agents WHERE id = ?;

-- name: GetAgentByUID :one
SELECT * FROM remote_agents WHERE uid = ?;

-- name: ListAgents :many
SELECT * FROM remote_agents ORDER BY name COLLATE NOCASE;

-- name: DeleteAgent :exec
DELETE FROM remote_agents WHERE id = ?;

-- name: SetAgentStatus :exec
UPDATE remote_agents SET status = ?, last_error = ?,
    last_seen = CASE WHEN ? = 'online' THEN unixepoch() ELSE last_seen END
WHERE id = ?;

-- name: CountAgentSources :one
SELECT count(*) FROM library_sources WHERE kind = 'agent' AND CAST(json_extract(config, '$.agent.agentId') AS INTEGER) = CAST(sqlc.arg(agent_id) AS INTEGER);
