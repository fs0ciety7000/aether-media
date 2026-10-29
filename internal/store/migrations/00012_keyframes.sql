-- +goose Up
-- Keyframe lists of media files (T-068): remuxing a video copy into HLS
-- needs them, and getting them can mean reading the whole file. Kept until
-- the file changes (version = "<size>:<mtime>") or disappears.
-- data: delta-encoded varints of the keyframe times in microseconds.
CREATE TABLE media_keyframes (
    media_file_id INTEGER PRIMARY KEY REFERENCES media_files(id) ON DELETE CASCADE,
    version       TEXT    NOT NULL,
    count         INTEGER NOT NULL,
    data          BLOB    NOT NULL,
    created_at    INTEGER NOT NULL
);

-- +goose Down
DROP TABLE media_keyframes;
