package jobs

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store"
)

func newRunner(t *testing.T) (*Runner, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "j.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewRunner(st, slog.New(slog.DiscardHandler)), st
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDedupAndRun(t *testing.T) {
	r, st := newRunner(t)
	ctx := context.Background()
	var ran atomic.Int32
	r.Register("probe", 2, func(context.Context, Job) error { ran.Add(1); return nil })

	added, _ := r.Enqueue(ctx, "probe", "file:1", nil, 0)
	dup, _ := r.Enqueue(ctx, "probe", "file:1", nil, 0)
	if !added || dup {
		t.Fatalf("added=%v dup=%v", added, dup)
	}
	r.Enqueue(ctx, "probe", "file:2", map[string]int{"id": 2}, 0)

	runCtx, cancel := context.WithCancel(ctx)
	if err := r.Start(runCtx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return ran.Load() == 2 })
	waitFor(t, func() bool { stats, _ := st.JobStats(ctx); return len(stats) == 0 })
	cancel()
	r.Wait()
}

func TestPermanentFailure(t *testing.T) {
	r, st := newRunner(t)
	ctx := context.Background()
	r.Register("x", 1, func(context.Context, Job) error { return ErrPermanent })
	r.Enqueue(ctx, "x", "k", nil, 0)
	runCtx, cancel := context.WithCancel(ctx)
	defer func() { cancel(); r.Wait() }()
	r.Start(runCtx)
	waitFor(t, func() bool {
		stats, _ := st.JobStats(ctx)
		return len(stats) == 1 && stats[0].Status == "failed"
	})
	// A failed job does not block a new one with the same key.
	if added, _ := r.Enqueue(ctx, "x", "k", nil, 0); !added {
		t.Fatal("re-enqueue after failure should be allowed")
	}
}

func TestRetryAndPanic(t *testing.T) {
	r, st := newRunner(t)
	ctx := context.Background()
	r.Register("boom", 1, func(context.Context, Job) error { panic("bad") })
	r.Register("flaky", 1, func(context.Context, Job) error { return errors.New("temporary") })
	r.Enqueue(ctx, "boom", "k", nil, 0)
	r.Enqueue(ctx, "flaky", "k", nil, 0)
	runCtx, cancel := context.WithCancel(ctx)
	defer func() { cancel(); r.Wait() }()
	r.Start(runCtx)
	waitFor(t, func() bool {
		stats, _ := st.JobStats(ctx)
		got := map[string]string{}
		for _, s := range stats {
			got[s.Kind] = s.Status
		}
		return got["boom"] == "failed" && got["flaky"] == "queued"
	})
}
