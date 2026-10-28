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

	"github.com/fs0ciety7000/aether-media/internal/sources"
)

// Watcher triggers incremental scans when library folders change, and on a
// fixed interval as a safety net (network shares often emit no events).
type Watcher struct {
	scanner  *Scanner
	debounce time.Duration
	interval time.Duration

	mu     sync.Mutex
	fsw    *fsnotify.Watcher
	roots  map[string]int64 // local source root → library id
	dirty  map[int64]*time.Timer
	ctx    context.Context
	agents map[int64]context.CancelFunc // agent source id → event subscription
	reset  chan time.Duration
}

func NewWatcher(s *Scanner, debounce, interval time.Duration) *Watcher {
	return &Watcher{scanner: s, debounce: debounce, interval: interval, dirty: map[int64]*time.Timer{}, agents: map[int64]context.CancelFunc{},
		reset: make(chan time.Duration, 1)}
}

// SetInterval changes the periodic rescan interval while running; 0
// disables periodic rescans (file-system events still trigger scans).
func (w *Watcher) SetInterval(d time.Duration) {
	for {
		select {
		case w.reset <- d:
			return
		default:
			select { // replace a pending, not yet applied value
			case <-w.reset:
			default:
			}
		}
	}
}

// never is a ticker interval long enough to mean "disabled".
const never = 100 * 365 * 24 * time.Hour

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

	every := w.interval
	if every <= 0 {
		every = never
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case d := <-w.reset:
			if d <= 0 {
				d = never
			}
			ticker.Reset(d)
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

// Sync (re)loads local roots and their directory watches, and (re)subscribes
// to change events of remote agents. SFTP sources rely on periodic scans.
func (w *Watcher) Sync(ctx context.Context) error {
	srcs, err := w.scanner.store.ListAllSources(ctx)
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
	for id, cancel := range w.agents {
		cancel()
		delete(w.agents, id)
	}
	w.roots = map[string]int64{}
	dirs := 0
	for _, src := range srcs {
		switch src.Kind {
		case sources.KindLocal:
			w.roots[src.Root] = src.LibraryID
			dirs += w.addTree(src.Root)
		case sources.KindAgent:
			subCtx, cancel := context.WithCancel(w.ctx)
			w.agents[src.ID] = cancel
			go w.followAgent(subCtx, src.ID, src.LibraryID)
		}
	}
	w.scanner.log.Debug("surveillance active", "local", len(w.roots), "dirs", dirs, "agents", len(w.agents))
	return nil
}

// followAgent turns an agent's change events into debounced scans.
func (w *Watcher) followAgent(ctx context.Context, sourceID, libID int64) {
	ch, err := w.scanner.Sources.Changes(ctx, sourceID)
	if err != nil {
		w.scanner.log.Warn("abonnement aux changements de l'agent impossible", "source", sourceID, "err", err)
		return
	}
	for range ch {
		w.mu.Lock()
		w.markDirty(libID)
		w.mu.Unlock()
	}
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
	w.markDirty(libID)
}

// markDirty schedules a debounced scan of a library. Callers hold w.mu.
func (w *Watcher) markDirty(libID int64) {
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
