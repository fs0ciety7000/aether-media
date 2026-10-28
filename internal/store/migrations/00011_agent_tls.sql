-- +goose Up
-- HTTPS agents: SHA-256 fingerprint (lowercase hex) of the certificate
-- pinned at pairing. Empty: plain HTTP agent (paired before HTTPS) or
-- certificate verified by the system CAs (agent behind a reverse proxy).
ALTER TABLE remote_agents ADD COLUMN fingerprint TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE remote_agents DROP COLUMN fingerprint;
