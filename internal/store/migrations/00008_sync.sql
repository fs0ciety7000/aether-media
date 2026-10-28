-- +goose Up
-- M6b: Trakt / Simkl synchronisation.

-- Application credentials entered by an administrator (configuration file
-- and environment take precedence). secret = encrypted {"clientId","clientSecret"}.
CREATE TABLE sync_apps (
    service    TEXT    PRIMARY KEY CHECK (service IN ('trakt', 'simkl')),
    secret     BLOB    NOT NULL,
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- One link per user and service.
CREATE TABLE sync_connections (
    id                 INTEGER PRIMARY KEY,
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    service            TEXT    NOT NULL CHECK (service IN ('trakt', 'simkl')),
    status             TEXT    NOT NULL DEFAULT 'pending', -- pending | connected | reauth_required
    secret             BLOB,                               -- encrypted {"access","refresh"}
    token_expires_at   INTEGER,
    account            TEXT    NOT NULL DEFAULT '',        -- remote user name
    remote_id          TEXT    NOT NULL DEFAULT '',        -- remote user id / slug
    features           TEXT    NOT NULL DEFAULT '{}',      -- {"history": true, ...}
    cursors            TEXT    NOT NULL DEFAULT '{}',      -- last remote activity seen, per feature
    -- Device code / PIN pairing in progress (independent of an existing link).
    pending_secret     BLOB,                               -- encrypted {"deviceCode"}
    pending_user_code  TEXT    NOT NULL DEFAULT '',
    pending_url        TEXT    NOT NULL DEFAULT '',
    pending_expires_at INTEGER,
    pending_interval   INTEGER NOT NULL DEFAULT 5,
    pending_polled_at  INTEGER,
    connected_at       INTEGER,
    last_sync_at       INTEGER,
    last_error         TEXT    NOT NULL DEFAULT '',
    last_error_at      INTEGER,
    created_at         INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at         INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE (user_id, service)
);

-- Last synchronised state, per connection and feature (three-way merge):
-- an item is present when it was on both sides after the previous sync.
CREATE TABLE sync_state (
    connection_id INTEGER NOT NULL REFERENCES sync_connections(id) ON DELETE CASCADE,
    feature       TEXT    NOT NULL,           -- history | progress | watchlist | ratings
    item_id       INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    value         REAL    NOT NULL DEFAULT 0, -- rating (1-10) or progress (%)
    remote_at     INTEGER,                    -- remote timestamp of the entry
    remote_ref    TEXT    NOT NULL DEFAULT '',-- remote entry id (Trakt playback id)
    PRIMARY KEY (connection_id, feature, item_id)
);

-- Journal shown to the user: conflicts, imports/exports, errors.
CREATE TABLE sync_events (
    id            INTEGER PRIMARY KEY,
    connection_id INTEGER NOT NULL REFERENCES sync_connections(id) ON DELETE CASCADE,
    kind          TEXT    NOT NULL,           -- conflict | error | import | export
    feature       TEXT    NOT NULL,
    item_id       INTEGER REFERENCES items(id) ON DELETE SET NULL,
    winner        TEXT    NOT NULL DEFAULT '',-- local | remote (conflicts)
    message       TEXT    NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX sync_events_connection ON sync_events(connection_id, id);

-- Per-user "to watch" list. Removals are kept (removed_at) so that they can
-- be propagated and win or lose conflicts by date.
CREATE TABLE user_watchlist (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_id    INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    added_at   INTEGER NOT NULL DEFAULT (unixepoch()),
    removed_at INTEGER,
    PRIMARY KEY (user_id, item_id)
);
CREATE INDEX user_watchlist_active ON user_watchlist(user_id, added_at) WHERE removed_at IS NULL;

-- When "played" or the rating last changed (conflict resolution).
ALTER TABLE user_item_data ADD COLUMN played_changed_at INTEGER;
ALTER TABLE user_item_data ADD COLUMN rated_at INTEGER;

-- +goose Down
ALTER TABLE user_item_data DROP COLUMN rated_at;
ALTER TABLE user_item_data DROP COLUMN played_changed_at;
DROP TABLE user_watchlist;
DROP TABLE sync_events;
DROP TABLE sync_state;
DROP TABLE sync_connections;
DROP TABLE sync_apps;
