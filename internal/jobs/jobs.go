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

// RetryAfterError asks for the next attempt to wait at least After (e.g. a
// remote service answered 429 with Retry-After). It still counts as an
// attempt, so a job never retries forever.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// RetryAfter wraps err so the job is retried no sooner than d.
func RetryAfter(err error, d time.Duration) error { return &RetryAfterError{Err: err, After: d} }

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

	mu      sync.Mutex
	running map[int64]*runningJob
	notify  func()
}

type runningJob struct {
	cancel    context.CancelFunc
	cancelled bool
}

func NewRunner(s *store.Store, log *slog.Logger) *Runner {
	return &Runner{store: s, log: log, queues: make(map[string]*queue), running: map[int64]*runningJob{}}
}

// SetNotify registers a function called (from any goroutine, possibly in
// bursts: debounce it) whenever the queue changes.
func (r *Runner) SetNotify(f func()) {
	r.mu.Lock()
	r.notify = f
	r.mu.Unlock()
}

func (r *Runner) changed() {
	if r == nil {
		return
	}
	r.mu.Lock()
	f := r.notify
	r.mu.Unlock()
	if f != nil {
		f()
	}
}

// Errors of Cancel and Retry.
var (
	ErrNotFound      = errors.New("jobs: job not found")
	ErrNotCancelable = errors.New("jobs: job cannot be cancelled")
	ErrNotRetryable  = errors.New("jobs: only failed jobs can be retried (or the same work is already pending)")
)

// Cancel stops a running job (its context is cancelled and the job is
// dropped once the handler returns), removes a queued one, or dismisses a
// failed one.
func (r *Runner) Cancel(ctx context.Context, id int64) error {
	r.mu.Lock()
	if rj, ok := r.running[id]; ok {
		rj.cancelled = true
		rj.cancel()
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	job, err := r.store.GetJob(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if job.Status == "running" { // claimed by another process (aether scan)
		return ErrNotCancelable
	}
	n, err := r.store.DeleteJobWithStatus(ctx, db.DeleteJobWithStatusParams{ID: id, Status: job.Status})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotCancelable // started meanwhile
	}
	r.log.Info("tâche annulée", "kind", job.Kind, "key", job.Key, "status", job.Status)
	r.changed()
	return nil
}

// Retry puts a failed job back in the queue with a fresh attempt budget.
func (r *Runner) Retry(ctx context.Context, id int64) error {
	job, err := r.store.GetJob(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	n, err := r.store.RetryFailedJob(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotRetryable
	}
	r.log.Info("tâche relancée", "kind", job.Kind, "key", job.Key)
	r.Wake(job.Kind)
	r.changed()
	return nil
}

// IsRunning reports whether this process is executing the job.
func (r *Runner) IsRunning(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.running[id]
	return ok
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

// EnqueueAfter is Enqueue for a job that may not start before delay has
// elapsed. While it waits, further requests with the same key are merged
// into it (debouncing).
func (r *Runner) EnqueueAfter(ctx context.Context, kind, key string, payload any, priority int, delay time.Duration) (bool, error) {
	return enqueue(ctx, r.store.Queries, kind, key, payload, priority, time.Now().Add(delay), r)
}

// Enqueue is usable inside a transaction (q from store.Tx). Pass the runner
// to wake its workers, or nil.
func Enqueue(ctx context.Context, q *db.Queries, kind, key string, payload any, priority int, r *Runner) (bool, error) {
	return enqueue(ctx, q, kind, key, payload, priority, time.Now(), r)
}

func enqueue(ctx context.Context, q *db.Queries, kind, key string, payload any, priority int, at time.Time, r *Runner) (bool, error) {
	data := []byte("{}")
	if payload != nil {
		var err error
		if data, err = json.Marshal(payload); err != nil {
			return false, err
		}
	}
	n, err := q.EnqueueJob(ctx, db.EnqueueJobParams{
		Kind: kind, Key: key, Payload: string(data), Priority: int64(priority), RunAfter: at.Unix(),
	})
	if err != nil {
		return false, fmt.Errorf("enqueue %s/%s: %w", kind, key, err)
	}
	if n > 0 && r != nil {
		r.Wake(kind)
		r.changed()
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
	jctx, cancel := context.WithCancel(ctx)
	rj := &runningJob{cancel: cancel}
	r.mu.Lock()
	r.running[j.ID] = rj
	r.mu.Unlock()
	r.changed()
	err := safeCall(jctx, h, j)
	r.mu.Lock()
	delete(r.running, j.ID)
	cancelled := rj.cancelled
	r.mu.Unlock()
	cancel()
	defer r.changed()
	// Bookkeeping must survive shutdown so the job is not lost mid-update.
	bg := context.WithoutCancel(ctx)
	switch {
	case cancelled && ctx.Err() == nil:
		r.log.Info("tâche annulée", "kind", j.Kind, "key", j.Key)
		err = r.store.DeleteJob(bg, j.ID)
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
		if ra, ok := errors.AsType[*RetryAfterError](err); ok {
			backoff = max(ra.After, time.Second)
		}
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
