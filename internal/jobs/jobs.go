// Package jobs is a small persistent job queue backed by SQLite. Jobs are
// deduplicated by (kind, key) while pending, survive restarts, and are retried
// with exponential backoff before being marked failed.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// Job is the unit of work handed to a Handler.
type Job struct {
	ID       int64
	Kind     string
	Key      string
	Payload  json.RawMessage
	Attempts int
}

// Handler processes one job. Returning an error schedules a retry, unless it
// wraps ErrPermanent.
type Handler func(ctx context.Context, j Job) error

// ErrPermanent marks a failure that retrying will not fix.
var ErrPermanent = errors.New("permanent failure")

const (
	maxAttempts  = 3
	pollInterval = 2 * time.Second
)

type queue struct {
	handler Handler
	workers int
	wake    chan struct{}
}

// Runner dispatches queued jobs to registered handlers.
type Runner struct {
	store  *store.Store
	log    *slog.Logger
	queues map[string]*queue
	wg     sync.WaitGroup
}

func NewRunner(s *store.Store, log *slog.Logger) *Runner {
	return &Runner{store: s, log: log, queues: make(map[string]*queue)}
}

// Register sets the handler for a job kind with the given concurrency. It
// must be called before Start.
func (r *Runner) Register(kind string, workers int, h Handler) {
	r.queues[kind] = &queue{handler: h, workers: max(1, workers), wake: make(chan struct{}, 1)}
}

// Enqueue adds a job unless one with the same kind and key is already
// pending. It reports whether a job was added.
func (r *Runner) Enqueue(ctx context.Context, kind, key string, payload any, priority int) (bool, error) {
	return Enqueue(ctx, r.store.Queries, kind, key, payload, priority, r)
}

// Enqueue is usable inside a transaction (q from store.Tx). Pass the runner
// to wake its workers, or nil.
func Enqueue(ctx context.Context, q *db.Queries, kind, key string, payload any, priority int, r *Runner) (bool, error) {
	data := []byte("{}")
	if payload != nil {
		var err error
		if data, err = json.Marshal(payload); err != nil {
			return false, err
		}
	}
	n, err := q.EnqueueJob(ctx, db.EnqueueJobParams{
		Kind: kind, Key: key, Payload: string(data), Priority: int64(priority), RunAfter: time.Now().Unix(),
	})
	if err != nil {
		return false, fmt.Errorf("enqueue %s/%s: %w", kind, key, err)
	}
	if n > 0 && r != nil {
		r.Wake(kind)
	}
	return n > 0, nil
}

// Wake nudges idle workers of a kind to poll immediately.
func (r *Runner) Wake(kind string) {
	if q, ok := r.queues[kind]; ok {
		select {
		case q.wake <- struct{}{}:
		default:
		}
	}
}

// Start recovers jobs interrupted by a previous shutdown and launches
// workers. They stop when ctx is cancelled; Wait blocks until they have.
func (r *Runner) Start(ctx context.Context) error {
	n, err := r.store.RequeueRunningJobs(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		r.log.Info("jobs interrompus remis en file", "count", n)
	}
	for kind, q := range r.queues {
		for range q.workers {
			r.wg.Add(1)
			go r.work(ctx, kind, q)
		}
	}
	return nil
}

func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) work(ctx context.Context, kind string, q *queue) {
	defer r.wg.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
		case <-timer.C:
		}
		// Drain the queue before going back to sleep.
		for ctx.Err() == nil {
			row, err := r.store.ClaimJob(ctx, kind)
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				if ctx.Err() == nil {
					r.log.Error("claim job", "kind", kind, "err", err)
				}
				break
			}
			r.run(ctx, q.handler, row)
		}
		timer.Reset(pollInterval)
	}
}

func (r *Runner) run(ctx context.Context, h Handler, row db.Job) {
	j := Job{ID: row.ID, Kind: row.Kind, Key: row.Key, Payload: json.RawMessage(row.Payload), Attempts: int(row.Attempts)}
	err := safeCall(ctx, h, j)
	// Bookkeeping must survive shutdown so the job is not lost mid-update.
	bg := context.WithoutCancel(ctx)
	switch {
	case err == nil:
		err = r.store.DeleteJob(bg, j.ID)
	case ctx.Err() != nil:
		// Shutting down: put it back untouched for the next start.
		err = r.store.RetryJob(bg, db.RetryJobParams{Error: "", RunAfter: time.Now().Unix(), ID: j.ID})
	case errors.Is(err, ErrPermanent) || j.Attempts >= maxAttempts:
		r.log.Warn("job en échec", "kind", j.Kind, "key", j.Key, "attempts", j.Attempts, "err", err)
		err = r.store.FailJob(bg, db.FailJobParams{Error: err.Error(), ID: j.ID})
	default:
		backoff := time.Duration(1<<j.Attempts) * 30 * time.Second
		r.log.Info("job réessayé", "kind", j.Kind, "key", j.Key, "in", backoff, "err", err)
		err = r.store.RetryJob(bg, db.RetryJobParams{Error: err.Error(), RunAfter: time.Now().Add(backoff).Unix(), ID: j.ID})
	}
	if err != nil {
		r.log.Error("job bookkeeping", "kind", j.Kind, "key", j.Key, "err", err)
	}
}

func safeCall(ctx context.Context, h Handler, j Job) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("%w: panic: %v", ErrPermanent, v)
		}
	}()
	return h(ctx, j)
}
