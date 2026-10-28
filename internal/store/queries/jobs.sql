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

-- name: ListJobsByStatus :many
-- Admin view of the queue, with a human label resolved from the job key.
SELECT j.id, j.kind, j.key, j.status, j.priority, j.attempts, j.error, j.run_after, j.created_at, j.updated_at,
    CAST(COALESCE(CASE j.kind
        WHEN 'library.scan' THEN (SELECT l.name FROM libraries l WHERE l.id = CAST(j.key AS INTEGER))
        WHEN 'metadata.refresh' THEN (SELECT i.title FROM items i WHERE i.id = CAST(j.key AS INTEGER))
        WHEN 'media.probe' THEN (SELECT mf.path FROM media_files mf WHERE mf.id = CAST(j.key AS INTEGER))
        WHEN 'image.fetch' THEN (SELECT it.title FROM images im JOIN items it ON it.id = im.item_id WHERE im.id = CAST(j.key AS INTEGER))
        WHEN 'sync.run' THEN (SELECT c.service || ':' || u.name FROM sync_connections c JOIN users u ON u.id = c.user_id
                              WHERE c.id = CAST(substr(j.key, 6) AS INTEGER))
    END, '') AS TEXT) AS label
FROM jobs j
WHERE j.status = sqlc.arg(status)
  AND (CAST(sqlc.narg(kind) AS TEXT) IS NULL OR j.kind = CAST(sqlc.narg(kind) AS TEXT))
ORDER BY CASE WHEN j.status = 'queued' THEN 0 ELSE 1 END,
         CASE WHEN j.status = 'queued' THEN -j.priority ELSE 0 END,
         CASE WHEN j.status = 'queued' THEN j.run_after ELSE -j.updated_at END,
         j.id
LIMIT CAST(sqlc.arg(lim) AS INTEGER);

-- name: GetJob :one
SELECT * FROM jobs WHERE id = ?;

-- name: RetryFailedJob :execrows
-- Requeues a failed job from scratch, unless the same work is already pending.
UPDATE jobs SET status = 'queued', attempts = 0, error = '', run_after = unixepoch(), updated_at = unixepoch()
WHERE jobs.id = sqlc.arg(id) AND jobs.status = 'failed'
  AND NOT EXISTS (SELECT 1 FROM jobs p WHERE p.kind = jobs.kind AND p.key = jobs.key AND p.status IN ('queued', 'running'));

-- name: DeleteJobWithStatus :execrows
DELETE FROM jobs WHERE id = sqlc.arg(id) AND status = sqlc.arg(status);
