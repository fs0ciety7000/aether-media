-- +goose Up
-- T-047: actor photos are fetched once into the image cache (content hash)
-- and served by Aether instead of linking the browser to the remote URL.
ALTER TABLE people ADD COLUMN image_hash TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE people DROP COLUMN image_hash;
