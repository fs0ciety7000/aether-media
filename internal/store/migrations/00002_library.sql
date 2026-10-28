-- +goose Up
-- Stable identity of an item across rescans: "movie:<file>", "series:<dir>",
-- "season:<dir>#<n>", "episode:<file>", "artist:<name>", "album:<dir>", "track:<file>".
ALTER TABLE items ADD COLUMN key TEXT;
ALTER TABLE items ADD COLUMN edition TEXT;
ALTER TABLE items ADD COLUMN air_date TEXT;
ALTER TABLE items ADD COLUMN index_number_end INTEGER;
CREATE UNIQUE INDEX items_key ON items(key);
DROP INDEX items_path;
CREATE INDEX items_path ON items(path);
CREATE INDEX items_missing ON items(missing, date_updated) WHERE missing = 1;

CREATE TABLE media_files (
    id           INTEGER PRIMARY KEY,
    item_id      INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    library_id   INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path         TEXT    NOT NULL UNIQUE,
    size         INTEGER NOT NULL,
    mtime        INTEGER NOT NULL,
    part         INTEGER NOT NULL DEFAULT 0,
    container    TEXT,
    duration_ms  INTEGER,
    bitrate      INTEGER,
    probed_at    INTEGER,
    probe_error  TEXT
);
CREATE INDEX media_files_item ON media_files(item_id);
CREATE INDEX media_files_library ON media_files(library_id);

CREATE TABLE media_streams (
    id                  INTEGER PRIMARY KEY,
    media_file_id       INTEGER NOT NULL REFERENCES media_files(id) ON DELETE CASCADE,
    idx                 INTEGER NOT NULL,
    type                TEXT    NOT NULL,
    codec               TEXT    NOT NULL DEFAULT '',
    profile             TEXT    NOT NULL DEFAULT '',
    level               INTEGER NOT NULL DEFAULT 0,
    width               INTEGER NOT NULL DEFAULT 0,
    height              INTEGER NOT NULL DEFAULT 0,
    frame_rate          REAL    NOT NULL DEFAULT 0,
    bit_depth           INTEGER NOT NULL DEFAULT 0,
    hdr                 TEXT    NOT NULL DEFAULT '',
    channels            INTEGER NOT NULL DEFAULT 0,
    channel_layout      TEXT    NOT NULL DEFAULT '',
    sample_rate         INTEGER NOT NULL DEFAULT 0,
    language            TEXT    NOT NULL DEFAULT '',
    title               TEXT    NOT NULL DEFAULT '',
    is_default          INTEGER NOT NULL DEFAULT 0,
    is_forced           INTEGER NOT NULL DEFAULT 0,
    is_hearing_impaired INTEGER NOT NULL DEFAULT 0,
    is_image_sub        INTEGER NOT NULL DEFAULT 0,
    is_external         INTEGER NOT NULL DEFAULT 0,
    external_path       TEXT    NOT NULL DEFAULT '',
    bitrate             INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX media_streams_file ON media_streams(media_file_id);

CREATE TABLE chapters (
    media_file_id  INTEGER NOT NULL REFERENCES media_files(id) ON DELETE CASCADE,
    idx            INTEGER NOT NULL,
    start_ms       INTEGER NOT NULL,
    end_ms         INTEGER NOT NULL,
    title          TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (media_file_id, idx)
);

CREATE TABLE jobs (
    id          INTEGER PRIMARY KEY,
    kind        TEXT    NOT NULL,
    key         TEXT    NOT NULL,
    payload     TEXT    NOT NULL DEFAULT '{}',
    status      TEXT    NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'failed')),
    priority    INTEGER NOT NULL DEFAULT 0,
    attempts    INTEGER NOT NULL DEFAULT 0,
    error       TEXT    NOT NULL DEFAULT '',
    run_after   INTEGER NOT NULL DEFAULT (unixepoch()),
    created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch())
);
-- At most one pending job per (kind, key): re-enqueueing is a no-op.
CREATE UNIQUE INDEX jobs_pending ON jobs(kind, key) WHERE status IN ('queued', 'running');
CREATE INDEX jobs_claim ON jobs(kind, status, priority DESC, id);

-- +goose Down
DROP TABLE jobs;
DROP TABLE chapters;
DROP TABLE media_streams;
DROP TABLE media_files;
DROP INDEX items_missing;
DROP INDEX items_path;
CREATE UNIQUE INDEX items_path ON items(path) WHERE path IS NOT NULL;
DROP INDEX items_key;
ALTER TABLE items DROP COLUMN index_number_end;
ALTER TABLE items DROP COLUMN air_date;
ALTER TABLE items DROP COLUMN edition;
ALTER TABLE items DROP COLUMN key;
