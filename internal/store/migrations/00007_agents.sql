-- +goose Up
-- Agents paired with this server (one row per machine); agent sources point
-- to them so several shares reuse the same pairing.
CREATE TABLE remote_agents (
    id          INTEGER PRIMARY KEY,
    uid         TEXT    NOT NULL UNIQUE,
    name        TEXT    NOT NULL DEFAULT '',
    url         TEXT    NOT NULL,
    secret      BLOB,                     -- encrypted {"token": ...}
    version     TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL DEFAULT 'unknown',
    last_error  TEXT    NOT NULL DEFAULT '',
    last_seen   INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Existing token-based agent sources become agents (uid unknown yet).
INSERT OR IGNORE INTO remote_agents (uid, name, url, secret, version)
SELECT 'legacy:' || json_extract(config, '$.agent.url'),
       COALESCE(json_extract(config, '$.agent.hostname'), json_extract(config, '$.agent.url')),
       json_extract(config, '$.agent.url'), secret, COALESCE(json_extract(config, '$.agent.version'), '')
FROM library_sources WHERE kind = 'agent' AND json_extract(config, '$.agent.url') IS NOT NULL
GROUP BY json_extract(config, '$.agent.url');

UPDATE library_sources SET config = json_set(config, '$.agent.agentId',
    (SELECT a.id FROM remote_agents a WHERE a.url = json_extract(library_sources.config, '$.agent.url')))
WHERE kind = 'agent';

-- +goose Down
UPDATE library_sources SET config = json_remove(config, '$.agent.agentId') WHERE kind = 'agent';
DROP TABLE remote_agents;
