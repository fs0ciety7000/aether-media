package library

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher triggers incremental scans when library folders change, and on a
// fixed interval as a safety net (network shares often emit no events).
type Watcher struct {
	scanner  *Scanner
	debounce time.Duration
	interval time.Duration

	mu    sync.Mutex
	fsw   *fsnotify.Watcher
	roots map[string]int64 // library root → library id
	dirty map[int64]*time.Timer
	ctx   context.Context
}

func NewWatcher(s *Scanner, debounce, interval time.Duration) *Watcher {
	return &Watcher{scanner: s, debounce: debounce, interval: interval, dirty: map[int64]*time.Timer{}}
}

// Run watches until ctx is cancelled. Call Sync after libraries change.
func (w *Watcher) Run(ctx context.Context) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = fsw.Close() }()
	w.mu.Lock()
	w.fsw, w.ctx = fsw, ctx
	w.mu.Unlock()
	if err := w.Sync(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			w.handle(ev)
		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			w.scanner.log.Warn("surveillance fichiers", "err", err)
		case <-ticker.C:
			w.scanAll(ctx)
		}
	}
}

// Sync (re)loads library roots and their directory watches.
func (w *Watcher) Sync(ctx context.Context) error {
	paths, err := w.scanner.store.ListAllLibraryPaths(ctx)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fsw == nil {
		return nil
	}
	for _, p := range w.fsw.WatchList() {
		_ = w.fsw.Remove(p)
	}
	w.roots = make(map[string]int64, len(paths))
	dirs := 0
	for _, p := range paths {
		w.roots[p.Path] = p.LibraryID
		dirs += w.addTree(p.Path)
	}
	w.scanner.log.Debug("surveillance active", "roots", len(paths), "dirs", dirs)
	return nil
}

// addTree watches root and its sub-directories (inotify is not recursive).
// Callers hold w.mu.
func (w *Watcher) addTree(root string) int {
	n := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != root && (strings.HasPrefix(d.Name(), ".") || skipDirs[strings.ToLower(d.Name())]) {
			return fs.SkipDir
		}
		if w.fsw.Add(p) == nil {
			n++
		}
		return nil
	})
	return n
}

func (w *Watcher) handle(ev fsnotify.Event) {
	if ev.Has(fsnotify.Chmod) && !ev.Has(fsnotify.Write) {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	libID, ok := w.libraryFor(ev.Name)
	if !ok {
		return
	}
	if ev.Has(fsnotify.Create) {
		if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
			w.addTree(ev.Name)
		}
	}
	// Debounce: a copy in progress emits many writes; scan once it settles.
	if t, ok := w.dirty[libID]; ok {
		t.Reset(w.debounce)
		return
	}
	w.dirty[libID] = time.AfterFunc(w.debounce, func() {
		w.mu.Lock()
		delete(w.dirty, libID)
		ctx := w.ctx
		w.mu.Unlock()
		if _, err := w.scanner.EnqueueScan(ctx, libID, false); err != nil && ctx.Err() == nil {
			w.scanner.log.Error("planification du scan", "library", libID, "err", err)
		}
	})
}

func (w *Watcher) libraryFor(p string) (int64, bool) {
	for root, id := range w.roots {
		if p == root || strings.HasPrefix(p, root+string(filepath.Separator)) {
			return id, true
		}
	}
	return 0, false
}

func (w *Watcher) scanAll(ctx context.Context) {
	libs, err := w.scanner.store.ListLibraries(ctx)
	if err != nil {
		return
	}
	for _, l := range libs {
		_, _ = w.scanner.EnqueueScan(ctx, l.ID, false)
	}
}
