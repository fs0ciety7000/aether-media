-- +goose Up
CREATE TABLE libraries (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL,
    kind        TEXT    NOT NULL CHECK (kind IN ('movies', 'shows', 'music', 'photos', 'mixed')),
    settings    TEXT    NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE library_paths (
    id          INTEGER PRIMARY KEY,
    library_id  INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path        TEXT    NOT NULL,
    UNIQUE (library_id, path)
);

CREATE TABLE items (
    id                  INTEGER PRIMARY KEY,
    library_id          INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    parent_id           INTEGER REFERENCES items(id) ON DELETE CASCADE,
    kind                TEXT    NOT NULL,
    title               TEXT    NOT NULL,
    sort_title          TEXT    NOT NULL,
    original_title      TEXT,
    year                INTEGER,
    overview            TEXT,
    path                TEXT,
    size                INTEGER,
    mtime               INTEGER,
    runtime_ms          INTEGER,
    index_number        INTEGER,
    parent_index_number INTEGER,
    provider_ids        TEXT    NOT NULL DEFAULT '{}',
    locked_fields       TEXT    NOT NULL DEFAULT '[]',
    missing             INTEGER NOT NULL DEFAULT 0,
    date_added          INTEGER NOT NULL DEFAULT (unixepoch()),
    date_updated        INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX items_parent ON items(parent_id);
CREATE INDEX items_library_kind ON items(library_id, kind, sort_title);
CREATE UNIQUE INDEX items_path ON items(path) WHERE path IS NOT NULL;

CREATE TABLE users (
    id             INTEGER PRIMARY KEY,
    name           TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    password_hash  TEXT    NOT NULL,
    is_admin       INTEGER NOT NULL DEFAULT 0,
    prefs          TEXT    NOT NULL DEFAULT '{}',
    created_at     INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE sessions (
    token_hash   BLOB    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name  TEXT    NOT NULL DEFAULT '',
    client       TEXT    NOT NULL DEFAULT '',
    last_seen    INTEGER NOT NULL DEFAULT (unixepoch()),
    created_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE items;
DROP TABLE library_paths;
DROP TABLE libraries;
