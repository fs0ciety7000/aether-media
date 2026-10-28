-- +goose Up
ALTER TABLE items ADD COLUMN tagline TEXT;
ALTER TABLE items ADD COLUMN premiere_date TEXT;
ALTER TABLE items ADD COLUMN end_date TEXT;
ALTER TABLE items ADD COLUMN status TEXT;
ALTER TABLE items ADD COLUMN official_rating TEXT;
ALTER TABLE items ADD COLUMN community_rating REAL;
ALTER TABLE items ADD COLUMN collection TEXT;
-- pending: never refreshed · matched: identified automatically · unmatched:
-- no confident match (needs the user) · manual: identified by the user ·
-- local: metadata from local files only (no remote provider available).
ALTER TABLE items ADD COLUMN identify_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE items ADD COLUMN metadata_refreshed_at INTEGER;
CREATE INDEX items_refresh ON items(library_id, metadata_refreshed_at);
CREATE INDEX items_identify ON items(identify_status) WHERE identify_status = 'unmatched';

CREATE TABLE images (
    id          INTEGER PRIMARY KEY,
    item_id     INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    type        TEXT    NOT NULL,
    idx         INTEGER NOT NULL DEFAULT 0,
    source      TEXT    NOT NULL,           -- remote URL or absolute local path
    is_local    INTEGER NOT NULL DEFAULT 0,
    provider    TEXT    NOT NULL DEFAULT '',
    language    TEXT    NOT NULL DEFAULT '',
    hash        TEXT,                       -- content hash in the image cache, NULL until fetched
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    blurhash    TEXT    NOT NULL DEFAULT '',
    dominant    TEXT    NOT NULL DEFAULT '',
    fetch_error TEXT    NOT NULL DEFAULT '',
    UNIQUE (item_id, type, idx)
);

CREATE TABLE genres (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE);
CREATE TABLE item_genres (
    item_id  INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    genre_id INTEGER NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (item_id, genre_id)
);
CREATE INDEX item_genres_genre ON item_genres(genre_id);

CREATE TABLE studios (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE);
CREATE TABLE item_studios (
    item_id   INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    studio_id INTEGER NOT NULL REFERENCES studios(id) ON DELETE CASCADE,
    PRIMARY KEY (item_id, studio_id)
);

CREATE TABLE people (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    -- Dedup key: "tmdb:<id>" when known, else "name:<lowercased name>".
    key          TEXT NOT NULL UNIQUE,
    provider_ids TEXT NOT NULL DEFAULT '{}',
    image_url    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE item_people (
    item_id   INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    kind      TEXT    NOT NULL,
    role      TEXT    NOT NULL DEFAULT '',
    ord       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (item_id, person_id, kind, role)
);
CREATE INDEX item_people_person ON item_people(person_id);

-- +goose Down
DROP TABLE item_people;
DROP TABLE people;
DROP TABLE item_studios;
DROP TABLE studios;
DROP TABLE item_genres;
DROP TABLE genres;
DROP TABLE images;
DROP INDEX items_identify;
DROP INDEX items_refresh;
ALTER TABLE items DROP COLUMN metadata_refreshed_at;
ALTER TABLE items DROP COLUMN identify_status;
ALTER TABLE items DROP COLUMN collection;
ALTER TABLE items DROP COLUMN community_rating;
ALTER TABLE items DROP COLUMN official_rating;
ALTER TABLE items DROP COLUMN status;
ALTER TABLE items DROP COLUMN end_date;
ALTER TABLE items DROP COLUMN premiere_date;
ALTER TABLE items DROP COLUMN tagline;
