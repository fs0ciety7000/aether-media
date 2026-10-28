// Package library discovers media files in library folders, maps them to an
// item tree (movies, series/seasons/episodes, artists/albums/tracks) and keeps
// the database in sync incrementally.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/naming"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// Job kinds handled by this package.
const (
	JobScan  = "library.scan"
	JobProbe = "media.probe"
)

const (
	batchSize = 200
	// Items missing for longer than this are deleted for good.
	missingGrace = 30 * 24 * time.Hour
)

// Scanner synchronises libraries with the file system.
type Scanner struct {
	store *store.Store
	jobs  *jobs.Runner
	hub   *events.Hub
	log   *slog.Logger
	// Sources resolves where library files live (local, agent, SFTP).
	Sources *sources.Manager
	// AfterScan, when set, runs after each successful scan (e.g. to queue
	// metadata refreshes for new items).
	AfterScan func(ctx context.Context, libID int64)
}

func NewScanner(s *store.Store, r *jobs.Runner, hub *events.Hub, log *slog.Logger, src *sources.Manager) *Scanner {
	return &Scanner{store: s, jobs: r, hub: hub, log: log, Sources: src}
}

// Result summarises a scan.
type Result struct {
	LibraryID   int64    `json:"libraryId"`
	Files       int      `json:"files"`
	Added       int      `json:"added"`
	Updated     int      `json:"updated"`
	Unchanged   int      `json:"unchanged"`
	Removed     int      `json:"removed"`
	Skipped     int      `json:"skipped"`
	Unavailable []string `json:"unavailable,omitempty"`
	DurationMs  int64    `json:"durationMs"`
}

type scanPayload struct {
	LibraryID int64 `json:"libraryId"`
	Full      bool  `json:"full,omitempty"`
}

// EnqueueScan schedules a scan of a library (deduplicated while pending).
// A full scan re-resolves unchanged files too.
func (s *Scanner) EnqueueScan(ctx context.Context, libID int64, full bool) (bool, error) {
	return s.jobs.Enqueue(ctx, JobScan, fmt.Sprint(libID), scanPayload{LibraryID: libID, Full: full}, 10)
}

// HandleScanJob is the jobs.Handler for JobScan.
func (s *Scanner) HandleScanJob(ctx context.Context, j jobs.Job) error {
	var p scanPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
	}
	_, err := s.Scan(ctx, p.LibraryID, p.Full)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // library deleted meanwhile
	}
	return err
}

// Scan synchronises one library and enqueues probe jobs for new or changed
// files.
func (s *Scanner) Scan(ctx context.Context, libID int64, full bool) (Result, error) {
	start := time.Now()
	res := Result{LibraryID: libID}
	lib, err := s.store.GetLibrary(ctx, libID)
	if err != nil {
		return res, err
	}
	srcs, err := s.store.ListSources(ctx, libID)
	if err != nil {
		return res, err
	}
	known, err := s.store.ListLibraryMediaFiles(ctx, libID)
	if err != nil {
		return res, err
	}
	existing := make(map[string]db.ListLibraryMediaFilesRow, len(known))
	for _, k := range known {
		existing[k.Path] = k
	}

	s.log.Info("scan démarré", "library", lib.Name, "kind", lib.Kind)
	s.hub.PublishAdmin("library.scan.started", map[string]any{"libraryId": libID, "name": lib.Name})

	var files []file
	var unavailable []string
	offline := map[int64]bool{}
	for _, src := range srcs {
		fsys, _, err := s.Sources.FS(ctx, src.ID)
		var found []file
		if err == nil {
			found, err = walk(ctx, fsys, src, func(name string) bool { return accepts(lib.Kind, name) })
		}
		if err != nil {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			s.log.Warn("source inaccessible, ses fichiers sont conservés", "source", src.Name, "err", err)
			unavailable = append(unavailable, src.Name)
			offline[src.ID] = true
			s.Sources.Forget(src.ID)
			s.Sources.SetStatus(ctx, src, err)
			continue
		}
		s.Sources.SetStatus(ctx, src, nil)
		files = append(files, found...)
	}
	res.Files = len(files)
	res.Unavailable = unavailable

	ids := make(map[string]int64) // item key → id, shared across batches
	seen := make(map[string]bool, len(files))
	for i := 0; i < len(files); i += batchSize {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		batch := files[i:min(i+batchSize, len(files))]
		err := s.store.Tx(ctx, func(q *db.Queries) error {
			for _, f := range batch {
				seen[f.path] = true
				if k, ok := existing[f.path]; ok && !full && k.Size == f.size && k.Mtime == f.mtime {
					res.Unchanged++
					continue
				}
				r, ok := resolve(libID, lib.Kind, f)
				if !ok {
					res.Skipped++
					continue
				}
				if err := s.upsertFile(ctx, q, libID, f, r, ids); err != nil {
					return fmt.Errorf("%s: %w", f.path, err)
				}
				if _, ok := existing[f.path]; ok {
					res.Updated++
				} else {
					res.Added++
				}
			}
			return nil
		})
		if err != nil {
			return res, err
		}
		s.hub.PublishAdmin("library.scan.progress", map[string]any{
			"libraryId": libID, "processed": min(i+batchSize, len(files)), "total": len(files),
		})
	}

	removed, err := s.removeVanished(ctx, libID, existing, seen, offline)
	if err != nil {
		return res, err
	}
	res.Removed = removed
	res.DurationMs = time.Since(start).Milliseconds()

	s.log.Info("scan terminé", "library", lib.Name, "files", res.Files, "added", res.Added,
		"updated", res.Updated, "unchanged", res.Unchanged, "removed", res.Removed,
		"skipped", res.Skipped, "duration_ms", res.DurationMs)
	// Public so members' lists refresh; the unavailable paths stay private.
	pub := res
	pub.Unavailable = nil
	s.hub.Publish("library.scan.finished", pub)
	if s.AfterScan != nil {
		s.AfterScan(ctx, libID)
	}
	return res, nil
}

// upsertFile writes the item chain and the media file, and enqueues a probe
// when the file content changed.
func (s *Scanner) upsertFile(ctx context.Context, q *db.Queries, libID int64, f file, r resolved, ids map[string]int64) error {
	var parent sql.NullInt64
	for i, n := range r.chain {
		isLeaf := i == len(r.chain)-1
		if id, ok := ids[n.key]; ok && !isLeaf {
			parent = sql.NullInt64{Int64: id, Valid: true}
			continue
		}
		id, err := q.UpsertItem(ctx, itemParams(libID, parent, n))
		if err != nil {
			return err
		}
		ids[n.key] = id
		parent = sql.NullInt64{Int64: id, Valid: true}
	}
	mf, err := q.UpsertMediaFile(ctx, db.UpsertMediaFileParams{
		ItemID: parent.Int64, LibraryID: libID, SourceID: sql.NullInt64{Int64: f.sourceID, Valid: f.sourceID != 0},
		Path: f.path, Size: f.size, Mtime: f.mtime, Part: int64(r.part),
	})
	if err != nil {
		return err
	}
	if !mf.ProbedAt.Valid {
		_, err = jobs.Enqueue(ctx, q, JobProbe, fmt.Sprint(mf.ID), probePayload{MediaFileID: mf.ID}, 0, s.jobs)
	}
	return err
}

func itemParams(libID int64, parent sql.NullInt64, n node) db.UpsertItemParams {
	ids := "{}"
	if n.ids != (naming.ProviderIDs{}) {
		m := map[string]string{}
		for k, v := range map[string]string{"tmdb": n.ids.TMDB, "imdb": n.ids.IMDB, "tvdb": n.ids.TVDB} {
			if v != "" {
				m[k] = v
			}
		}
		b, _ := json.Marshal(m)
		ids = string(b)
	}
	return db.UpsertItemParams{
		Key:               sql.NullString{String: n.key, Valid: true},
		LibraryID:         libID,
		ParentID:          parent,
		Kind:              n.kind,
		Title:             n.title,
		SortTitle:         sortKey(n),
		Year:              nullInt(n.year),
		Path:              nullStr(n.path),
		IndexNumber:       nullIntp(n.index),
		IndexNumberEnd:    nullIntp(n.indexEnd),
		ParentIndexNumber: nullIntp(n.parentIndex),
		Edition:           nullStr(n.edition),
		AirDate:           nullStr(n.airDate),
		ProviderIds:       ids,
	}
}

// sortKey orders episodes, seasons and tracks by number, everything else by
// title without leading articles.
func sortKey(n node) string {
	switch {
	case n.kind == "track" || n.kind == "episode":
		disc := 0
		if n.parentIndex != nil {
			disc = *n.parentIndex
		}
		num := 0
		if n.index != nil {
			num = *n.index
		}
		if n.airDate != "" && n.index == nil {
			return fmt.Sprintf("%04d-%s", disc, n.airDate)
		}
		return fmt.Sprintf("%04d-%05d-%s", disc, num, naming.SortTitle(n.title))
	case n.kind == "season" && n.index != nil:
		return fmt.Sprintf("%04d", *n.index)
	}
	return naming.SortTitle(n.title)
}

// removeVanished deletes media files no longer on disk (except under
// unavailable roots), flags items left without files as missing, and purges
// items missing for longer than the grace period.
func (s *Scanner) removeVanished(ctx context.Context, libID int64, existing map[string]db.ListLibraryMediaFilesRow, seen map[string]bool, offline map[int64]bool) (int, error) {
	removed := 0
	err := s.store.Tx(ctx, func(q *db.Queries) error {
		for p, k := range existing {
			if seen[p] || offline[k.SourceID.Int64] {
				continue
			}
			if err := q.DeleteMediaFile(ctx, k.ID); err != nil {
				return err
			}
			removed++
		}
		if _, err := q.MarkOrphansMissing(ctx, libID); err != nil {
			return err
		}
		// Seasons first, then series/albums, then artists.
		for range 3 {
			n, err := q.MarkEmptyContainersMissing(ctx, libID)
			if err != nil {
				return err
			}
			if n == 0 {
				break
			}
		}
		_, err := q.PurgeMissingItems(ctx, time.Now().Add(-missingGrace).Unix())
		return err
	})
	return removed, err
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func nullInt(v int) sql.NullInt64     { return sql.NullInt64{Int64: int64(v), Valid: v != 0} }
func nullIntp(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}
