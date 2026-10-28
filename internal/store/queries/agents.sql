-- name: UpsertAgent :one
INSERT INTO remote_agents (uid, name, url, secret, version, status, last_seen)
VALUES (?, ?, ?, ?, ?, 'online', unixepoch())
ON CONFLICT (uid) DO UPDATE SET
    name = excluded.name, url = excluded.url, secret = excluded.secret, version = excluded.version,
    status = 'online', last_error = '', last_seen = unixepoch()
RETURNING *;

-- name: GetAgent :one
SELECT * FROM remote_agents WHERE id = ?;

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
