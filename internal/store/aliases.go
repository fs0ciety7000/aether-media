package store

import (
	"context"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// Re-exported parameter types so callers need not import package db.
type (
	CreateLibraryParams = db.CreateLibraryParams
)

// AddLocalSource attaches a local folder to a library (CLI and tests).
func (s *Store) AddLocalSource(ctx context.Context, libID int64, path string) (db.LibrarySource, error) {
	return s.CreateSource(ctx, db.CreateSourceParams{LibraryID: libID, Kind: "local", Name: path, Root: path, Config: "{}"})
}
