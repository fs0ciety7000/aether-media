package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// TestConcurrentWritersNoLock reproduces T-089: a metadata refresh (several
// workers, read-then-write transactions) running alongside a scan (batched
// write transactions) and the job queue (autocommit writes), while the API
// reads. With BEGIN DEFERRED transactions, a transaction that read first and
// then tries to write fails at once with "database is locked"
// (SQLITE_BUSY_SNAPSHOT: busy_timeout does not apply to a lock upgrade).
// Transactions must start as writers (BEGIN IMMEDIATE, single write
// connection) so that they queue instead of failing, and no update is lost.
func TestConcurrentWritersNoLock(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test")
	}
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "aether.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const (
		txWorkers   = 8  // refresh + image + probe workers
		txIters     = 60 // read-modify-write transactions per worker
		autoWorkers = 4  // job queue updates outside transactions
		autoIters   = 150
		readers     = 4
	)
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	fail := func(err error) {
		mu.Lock()
		errs = append(errs, err)
		mu.Unlock()
	}
	stop := make(chan struct{})

	// Read-then-write transactions on a shared counter (refresher.save reads
	// the item, then writes metadata, credits and images).
	for w := range txWorkers {
		wg.Go(func() {
			for i := range txIters {
				err := s.Tx(ctx, func(q *db.Queries) error {
					cur := 0
					row, err := q.GetSetting(ctx, "stress.counter")
					switch {
					case errors.Is(err, sql.ErrNoRows):
					case err != nil:
						return err
					default:
						cur, _ = strconv.Atoi(row.Value)
					}
					time.Sleep(200 * time.Microsecond) // widen the read → write window
					if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: "stress.counter", Value: strconv.Itoa(cur + 1)}); err != nil {
						return err
					}
					// A scan batch writes many rows in one transaction.
					return q.UpsertSetting(ctx, db.UpsertSettingParams{Key: fmt.Sprintf("stress.w%d", w), Value: strconv.Itoa(i)})
				})
				if err != nil {
					fail(fmt.Errorf("tx worker %d iter %d: %w", w, i, err))
					return
				}
			}
		})
	}
	// Autocommit writes (job claims, status updates) through the main pool.
	for w := range autoWorkers {
		wg.Go(func() {
			for i := range autoIters {
				if err := s.UpsertSetting(ctx, db.UpsertSettingParams{Key: fmt.Sprintf("stress.a%d", w), Value: strconv.Itoa(i)}); err != nil {
					fail(fmt.Errorf("autocommit worker %d iter %d: %w", w, i, err))
					return
				}
			}
		})
	}
	// API reads.
	var rg sync.WaitGroup
	for range readers {
		rg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := s.ListSettings(ctx); err != nil {
					fail(fmt.Errorf("reader: %w", err))
					return
				}
			}
		})
	}
	wg.Wait()
	close(stop)
	rg.Wait()

	for i, err := range errs {
		if i >= 5 {
			t.Errorf("… %d errors in total", len(errs))
			break
		}
		t.Error(err)
	}
	row, err := s.GetSetting(ctx, "stress.counter")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := row.Value, strconv.Itoa(txWorkers*txIters); got != want && len(errs) == 0 {
		t.Fatalf("counter = %s, want %s (lost update)", got, want)
	}
}
