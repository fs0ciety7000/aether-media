-- name: EnqueueJob :execrows
INSERT INTO jobs (kind, key, payload, priority, run_after)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (kind, key) WHERE status IN ('queued', 'running') DO NOTHING;

-- name: ClaimJob :one
UPDATE jobs SET status = 'running', attempts = attempts + 1, updated_at = unixepoch()
WHERE id = (
    SELECT j.id FROM jobs j
    WHERE j.kind = ? AND j.status = 'queued' AND j.run_after <= unixepoch()
    ORDER BY j.priority DESC, j.id
    LIMIT 1
)
RETURNING *;

-- name: DeleteJob :exec
DELETE FROM jobs WHERE id = ?;

-- name: RetryJob :exec
UPDATE jobs SET status = 'queued', error = ?, run_after = ?, updated_at = unixepoch() WHERE id = ?;

-- name: FailJob :exec
UPDATE jobs SET status = 'failed', error = ?, updated_at = unixepoch() WHERE id = ?;

-- name: RequeueRunningJobs :execrows
UPDATE jobs SET status = 'queued', updated_at = unixepoch() WHERE status = 'running';

-- name: JobStats :many
SELECT kind, status, count(*) AS n FROM jobs GROUP BY kind, status ORDER BY kind, status;

-- name: PurgeFailedJobs :execrows
DELETE FROM jobs WHERE status = 'failed' AND updated_at < ?;
