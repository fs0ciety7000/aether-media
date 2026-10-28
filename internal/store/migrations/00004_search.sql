-- +goose Up
-- Full-text index over item titles, accent- and case-insensitive, with
-- prefix indexes for instant search.
CREATE VIRTUAL TABLE items_fts USING fts5(
    title, original_title,
    content = 'items', content_rowid = 'id',
    tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3'
);

-- +goose StatementBegin
CREATE TRIGGER items_fts_ai AFTER INSERT ON items BEGIN
    INSERT INTO items_fts (rowid, title, original_title) VALUES (new.id, new.title, COALESCE(new.original_title, ''));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER items_fts_ad AFTER DELETE ON items BEGIN
    INSERT INTO items_fts (items_fts, rowid, title, original_title) VALUES ('delete', old.id, old.title, COALESCE(old.original_title, ''));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER items_fts_au AFTER UPDATE OF title, original_title ON items BEGIN
    INSERT INTO items_fts (items_fts, rowid, title, original_title) VALUES ('delete', old.id, old.title, COALESCE(old.original_title, ''));
    INSERT INTO items_fts (rowid, title, original_title) VALUES (new.id, new.title, COALESCE(new.original_title, ''));
END;
-- +goose StatementEnd

INSERT INTO items_fts (items_fts) VALUES ('rebuild');

CREATE VIRTUAL TABLE people_fts USING fts5(
    name, content = 'people', content_rowid = 'id',
    tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3'
);

-- +goose StatementBegin
CREATE TRIGGER people_fts_ai AFTER INSERT ON people BEGIN
    INSERT INTO people_fts (rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER people_fts_ad AFTER DELETE ON people BEGIN
    INSERT INTO people_fts (people_fts, rowid, name) VALUES ('delete', old.id, old.name);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER people_fts_au AFTER UPDATE OF name ON people BEGIN
    INSERT INTO people_fts (people_fts, rowid, name) VALUES ('delete', old.id, old.name);
    INSERT INTO people_fts (rowid, name) VALUES (new.id, new.name);
END;
-- +goose StatementEnd

INSERT INTO people_fts (people_fts) VALUES ('rebuild');

-- +goose Down
DROP TRIGGER people_fts_au;
DROP TRIGGER people_fts_ad;
DROP TRIGGER people_fts_ai;
DROP TABLE people_fts;
DROP TRIGGER items_fts_au;
DROP TRIGGER items_fts_ad;
DROP TRIGGER items_fts_ai;
DROP TABLE items_fts;
