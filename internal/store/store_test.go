package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenMigratesAndQueries(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "aether.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	lib, err := s.CreateLibrary(ctx, CreateLibraryParams{Name: "Films", Kind: "movies"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddLibraryPath(ctx, AddLibraryPathParams{LibraryID: lib.ID, Path: "/media/films"}); err != nil {
		t.Fatal(err)
	}
	libs, err := s.ListLibraries(ctx)
	if err != nil || len(libs) != 1 || libs[0].Name != "Films" {
		t.Fatalf("ListLibraries = %v, %v", libs, err)
	}
	s.Close()

	// Re-opening must be idempotent (no migration re-applied, data kept).
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if n, err := s.CountUsers(ctx); err != nil || n != 0 {
		t.Fatalf("CountUsers = %d, %v", n, err)
	}
	if _, err := s.CreateLibrary(ctx, CreateLibraryParams{Name: "x", Kind: "bogus"}); err == nil {
		t.Fatal("expected CHECK constraint to reject unknown kind")
	}
}
