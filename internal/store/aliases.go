package store

import "github.com/fs0ciety7000/aether-media/internal/store/db"

// Re-exported parameter types so callers need not import package db.
type (
	CreateLibraryParams  = db.CreateLibraryParams
	AddLibraryPathParams = db.AddLibraryPathParams
)
