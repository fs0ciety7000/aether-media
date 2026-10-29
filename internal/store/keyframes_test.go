package store

import (
	"context"
	"math"
	"path/filepath"
	"testing"
)

func TestKeyframesRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO libraries (id, name, kind) VALUES (1, 'Films', 'movies');
		INSERT INTO items (id, library_id, kind, title, sort_title) VALUES (1, 1, 'movie', 'Heat', 'heat');
		INSERT INTO media_files (id, item_id, library_id, path, size, mtime) VALUES (7, 1, 1, '/m/heat.mkv', 10, 20);`); err != nil {
		t.Fatal(err)
	}
	kf := []float64{-0.007, 0, 1.001, 2.002, 3.5035, 7200.25}
	if err := s.SaveKeyframes(ctx, 7, "10:20", kf); err != nil {
		t.Fatal(err)
	}
	got, ok := s.LoadKeyframes(ctx, 7, "10:20")
	if !ok || len(got) != len(kf) {
		t.Fatalf("load = %v, %v", got, ok)
	}
	for i := range kf {
		if math.Abs(got[i]-kf[i]) > 1e-6 {
			t.Fatalf("[%d] %v != %v", i, got[i], kf[i])
		}
	}
	if _, ok := s.LoadKeyframes(ctx, 7, "11:20"); ok {
		t.Fatal("stale version returned")
	}
	// Deleting the media file drops its keyframes.
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM media_files WHERE id = 7`); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM media_keyframes`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows left", n)
	}
}
