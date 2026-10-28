// Package store owns the SQLite database: connection setup, migrations and
// the sqlc-generated queries (package db).
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver

	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store bundles the raw connection and the typed queries.
//
// Two pools share the database file (T-089):
//   - DB (and the embedded Queries): reads and single-statement writes, any
//     number of connections (WAL lets readers run alongside the writer);
//   - a write pool of exactly one connection, used by Tx. Transactions start
//     with BEGIN IMMEDIATE, so a transaction that reads before it writes never
//     has to upgrade its lock: with BEGIN DEFERRED, that upgrade fails at once
//     with "database is locked" (SQLITE_BUSY_SNAPSHOT, busy_timeout does not
//     apply) as soon as another connection committed in between. Concurrent
//     Tx calls queue on the pool (in order, honouring ctx) instead of
//     spinning on the SQLite busy handler.
//
// A Tx body must use the q it is given, never the Store itself, for writes:
// the write lock is held until it returns.
type Store struct {
	DB *sql.DB
	*db.Queries

	w *sql.DB // write pool, one connection

	gen    atomic.Uint64 // bumped after each committed Tx (facet cache)
	facets facetCache
}

// busyTimeout bounds how long a single-statement write on DB waits for the
// write transaction in progress.
const busyTimeout = 15 * time.Second

func dsn(path string) string {
	q := url.Values{}
	q.Set("_txlock", "immediate")
	for _, p := range []string{
		fmt.Sprintf("busy_timeout(%d)", busyTimeout.Milliseconds()),
		"journal_mode(WAL)",
		"foreign_keys(1)",
		"synchronous(NORMAL)",
	} {
		q.Add("_pragma", p)
	}
	return "file:" + path + "?" + q.Encode()
}

// Open opens (or creates) the database at path and applies pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	conn, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := migrate(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	w, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)
	w.SetConnMaxLifetime(0)
	w.SetConnMaxIdleTime(0)
	if err := w.PingContext(ctx); err != nil {
		conn.Close()
		w.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	return &Store{DB: conn, Queries: db.New(conn), w: w}, nil
}

func migrate(ctx context.Context, conn *sql.DB) error {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, sub)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	for _, r := range results {
		slog.Info("migration applied", "version", r.Source.Version, "path", r.Source.Path)
	}
	return nil
}

// Close closes both pools.
func (s *Store) Close() error { return errors.Join(s.w.Close(), s.DB.Close()) }

// Tx runs fn inside a write transaction (BEGIN IMMEDIATE on the single write
// connection), committing on success. Transactions are serialised: keep fn
// short and do slow work (network, ffprobe) before calling Tx.
func (s *Store) Tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(s.WithTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	err = tx.Commit()
	s.gen.Add(1)
	return err
}

// facetCache keeps aggregate queries (genre counts, filter facets) that scan
// a whole library. An entry is valid while no write transaction committed
// since it was computed (scans, probes and metadata refreshes all write
// through Tx) and for at most facetTTL (writes made outside Tx).
type facetCache struct {
	mu sync.Mutex
	m  map[string]facetEntry
}

type facetEntry struct {
	gen uint64
	at  time.Time
	v   any
}

const facetTTL = 2 * time.Minute

// InvalidateFacets drops cached aggregates (after a write made outside Tx
// that changes them, e.g. deleting a library).
func (s *Store) InvalidateFacets() { s.gen.Add(1) }

func cachedFacet[T any](s *Store, key string, compute func() (T, error)) (T, error) {
	gen := s.gen.Load()
	s.facets.mu.Lock()
	e, ok := s.facets.m[key]
	s.facets.mu.Unlock()
	if ok && e.gen == gen && time.Since(e.at) < facetTTL {
		return e.v.(T), nil
	}
	v, err := compute()
	if err != nil {
		return v, err
	}
	s.facets.mu.Lock()
	if s.facets.m == nil || len(s.facets.m) > 256 {
		s.facets.m = map[string]facetEntry{}
	}
	s.facets.m[key] = facetEntry{gen: gen, at: time.Now(), v: v}
	s.facets.mu.Unlock()
	return v, nil
}
