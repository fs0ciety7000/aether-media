-- +goose Up
-- M7: server settings edited from the admin UI and the first-run assistant.
-- The configuration file and the environment take precedence over them.
-- value = JSON; secret = encrypted JSON (internal/secret) for credentials
-- such as the TMDB key, never returned in clear by the API.
CREATE TABLE settings (
    key        TEXT    PRIMARY KEY,
    value      TEXT    NOT NULL DEFAULT 'null',
    secret     BLOB,
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Servers set up before M7 have nothing left to configure in the assistant.
INSERT INTO settings (key, value)
SELECT 'setup.wizard', '{"step":"done","completed":true}'
WHERE EXISTS (SELECT 1 FROM users);

-- Admin "Tasks" view: recent failures first.
CREATE INDEX jobs_status_updated ON jobs(status, updated_at DESC);

-- +goose Down
DROP INDEX jobs_status_updated;
DROP TABLE settings;
