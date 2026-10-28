-- +goose Up
CREATE TABLE library_sources (
    id          INTEGER PRIMARY KEY,
    library_id  INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    kind        TEXT    NOT NULL CHECK (kind IN ('local', 'agent', 'sftp')),
    name        TEXT    NOT NULL DEFAULT '',
    root        TEXT    NOT NULL,           -- local: absolute path · agent: sub-path in share · sftp: remote path
    config      TEXT    NOT NULL DEFAULT '{}', -- non-secret settings (JSON)
    secret      BLOB,                       -- encrypted credentials (internal/secret)
    status      TEXT    NOT NULL DEFAULT 'unknown' CHECK (status IN ('online', 'offline', 'unknown')),
    last_error  TEXT    NOT NULL DEFAULT '',
    last_seen   INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX library_sources_library ON library_sources(library_id);

INSERT INTO library_sources (library_id, kind, name, root)
SELECT library_id, 'local', path, path FROM library_paths;

ALTER TABLE media_files ADD COLUMN source_id INTEGER REFERENCES library_sources(id) ON DELETE CASCADE;
UPDATE media_files SET source_id = (
    SELECT s.id FROM library_sources s
    WHERE s.library_id = media_files.library_id
      AND substr(media_files.path, 1, length(s.root) + 1) = s.root || '/'
    ORDER BY length(s.root) DESC LIMIT 1
);
CREATE INDEX media_files_source ON media_files(source_id);

DROP TABLE library_paths;

-- +goose Down
CREATE TABLE library_paths (
    id          INTEGER PRIMARY KEY,
    library_id  INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path        TEXT    NOT NULL,
    UNIQUE (library_id, path)
);
INSERT INTO library_paths (library_id, path) SELECT library_id, root FROM library_sources WHERE kind = 'local';
DROP INDEX media_files_source;
ALTER TABLE media_files DROP COLUMN source_id;
DROP TABLE library_sources;
