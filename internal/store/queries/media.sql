-- name: ListLibraryMediaFiles :many
SELECT id, item_id, path, size, mtime FROM media_files WHERE library_id = ?;

-- name: UpsertMediaFile :one
INSERT INTO media_files (item_id, library_id, path, size, mtime, part)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (path) DO UPDATE SET
    item_id = excluded.item_id,
    size    = excluded.size,
    mtime   = excluded.mtime,
    part    = excluded.part,
    probed_at = CASE WHEN media_files.size = excluded.size AND media_files.mtime = excluded.mtime
                     THEN media_files.probed_at ELSE NULL END
RETURNING id, probed_at;

-- name: DeleteMediaFile :exec
DELETE FROM media_files WHERE id = ?;

-- name: GetMediaFile :one
SELECT mf.*, i.kind AS item_kind FROM media_files mf JOIN items i ON i.id = mf.item_id WHERE mf.id = ?;

-- name: ListItemMediaFiles :many
SELECT * FROM media_files WHERE item_id = ? ORDER BY part, path;

-- name: SetProbeResult :exec
UPDATE media_files SET container = ?, duration_ms = ?, bitrate = ?, probed_at = unixepoch(), probe_error = NULL WHERE id = ?;

-- name: SetProbeError :exec
UPDATE media_files SET probed_at = unixepoch(), probe_error = ? WHERE id = ?;

-- name: DeleteMediaStreams :exec
DELETE FROM media_streams WHERE media_file_id = ? AND is_external = 0;

-- name: InsertMediaStream :exec
INSERT INTO media_streams (
    media_file_id, idx, type, codec, profile, level, width, height, frame_rate, bit_depth,
    hdr, channels, channel_layout, sample_rate, language, title, is_default, is_forced,
    is_hearing_impaired, is_image_sub, bitrate
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListMediaStreams :many
SELECT * FROM media_streams WHERE media_file_id = ? ORDER BY is_external, idx;

-- name: DeleteChapters :exec
DELETE FROM chapters WHERE media_file_id = ?;

-- name: InsertChapter :exec
INSERT INTO chapters (media_file_id, idx, start_ms, end_ms, title) VALUES (?, ?, ?, ?, ?);
