package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
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
	if _, err := s.AddLocalSource(ctx, lib.ID, "/media/films"); err != nil {
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

func TestFacetCacheInvalidatedByTx(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "aether.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	calls := 0
	get := func() int {
		v, err := cachedFacet(s, "k", func() (int, error) { calls++; return calls, nil })
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if first, second := get(), get(); first != 1 || second != 1 {
		t.Fatal("not cached")
	}
	if err := s.Tx(ctx, func(*db.Queries) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if get() != 2 {
		t.Fatal("a committed Tx must invalidate")
	}
	s.InvalidateFacets()
	if get() != 3 {
		t.Fatal("InvalidateFacets must invalidate")
	}
}
