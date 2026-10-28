-- +goose Up
ALTER TABLE users ADD COLUMN all_libraries INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN max_rating_age INTEGER;           -- NULL = no parental limit
ALTER TABLE users ADD COLUMN block_unrated INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN last_seen INTEGER;

CREATE TABLE user_library_access (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, library_id)
);

ALTER TABLE sessions ADD COLUMN id INTEGER;
CREATE UNIQUE INDEX sessions_id ON sessions(id);
CREATE INDEX sessions_user ON sessions(user_id);

-- Age derived from official_rating (see internal/auth.RatingAge), for parental filtering.
ALTER TABLE items ADD COLUMN rating_age INTEGER;

CREATE TABLE user_item_data (
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_id        INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    position_ms    INTEGER NOT NULL DEFAULT 0,
    played         INTEGER NOT NULL DEFAULT 0,
    play_count     INTEGER NOT NULL DEFAULT 0,
    last_played_at INTEGER,
    is_favorite    INTEGER NOT NULL DEFAULT 0,
    rating         REAL,
    updated_at     INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (user_id, item_id)
);
CREATE INDEX user_item_data_progress ON user_item_data(user_id, last_played_at) WHERE position_ms > 0;

CREATE TABLE collections (
    id         INTEGER PRIMARY KEY,
    owner_id   INTEGER REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    shared     INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE TABLE collection_items (
    collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    item_id       INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    ord           INTEGER NOT NULL DEFAULT 0,
    added_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (collection_id, item_id)
);

-- +goose Down
DROP TABLE collection_items;
DROP TABLE collections;
DROP TABLE user_item_data;
ALTER TABLE items DROP COLUMN rating_age;
DROP INDEX sessions_user;
DROP INDEX sessions_id;
ALTER TABLE sessions DROP COLUMN id;
DROP TABLE user_library_access;
ALTER TABLE users DROP COLUMN last_seen;
ALTER TABLE users DROP COLUMN block_unrated;
ALTER TABLE users DROP COLUMN max_rating_age;
ALTER TABLE users DROP COLUMN all_libraries;
