package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	maxEventPaths   = 500
	subscriberQueue = 64
)

// changeEvent is the payload of an SSE "change" event.
type changeEvent struct {
	Share string   `json:"share"`
	Paths []string `json:"paths"`
}

// hub fans change events out to SSE subscribers. A subscriber whose queue is
// full is disconnected (its channel closed) so it reconnects and resyncs
// rather than silently missing changes.
type hub struct {
	mu     sync.Mutex
	subs   map[chan []byte]struct{}
	closed bool
}

func newHub() *hub { return &hub{subs: make(map[chan []byte]struct{})} }

func (h *hub) subscribe() (chan []byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, false
	}
	ch := make(chan []byte, subscriberQueue)
	h.subs[ch] = struct{}{}
	return ch, true
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

func (h *hub) publish(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func (h *hub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

// ---- /events ----

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ch, ok := s.hub.subscribe()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "shutting_down", "arrêt en cours")
		return
	}
	defer s.hub.unsubscribe(ch)
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(": ok\n\n")); err != nil {
		return
	}
	if rc.Flush() != nil {
		return
	}
	ping := time.NewTicker(s.pingEvery)
	defer ping.Stop()
	for {
		var err error
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, err = w.Write(msg)
		case <-ping.C:
			_, err = w.Write([]byte(": ping\n\n"))
		}
		if err == nil {
			err = rc.Flush()
		}
		if err != nil {
			return
		}
	}
}

// ---- watcher ----

type pending struct {
	paths    map[string]struct{}
	first    time.Time
	deadline time.Time
	timer    *time.Timer
}

// watch runs the fsnotify watcher until ctx is done.
func (s *Server) watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	// Publish the watcher before taking the snapshot: a share added
	// concurrently is then either in the snapshot or added by watchShare.
	s.watchMu.Lock()
	s.watcher = w
	s.watchMu.Unlock()
	defer func() {
		s.watchMu.Lock()
		s.watcher = nil
		s.watchMu.Unlock()
	}()
	for _, sh := range s.activeShares() {
		s.addRecursive(w, sh.path)
	}

	debounce := s.debounce
	maxWait := 6 * debounce // keep long copies from starving notifications
	flushc := make(chan string, 16)
	pend := make(map[string]*pending)
	defer func() {
		for _, p := range pend {
			p.timer.Stop()
		}
	}()

	record := func(sh *share, rel string) {
		p := pend[sh.name]
		now := time.Now()
		if p == nil {
			name := sh.name
			p = &pending{paths: make(map[string]struct{}), first: now}
			p.timer = time.AfterFunc(debounce, func() {
				select {
				case flushc <- name:
				case <-ctx.Done():
				}
			})
			pend[sh.name] = p
		}
		p.paths[rel] = struct{}{}
		dl := now.Add(debounce)
		if lim := p.first.Add(maxWait); dl.After(lim) {
			dl = lim
		}
		p.deadline = dl
		p.timer.Reset(time.Until(dl))
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			s.log.Warn("erreur de surveillance des fichiers", "err", err)
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op == fsnotify.Chmod {
				continue
			}
			matches := s.sharesFor(ev.Name)
			if len(matches) == 0 {
				continue
			}
			if ev.Has(fsnotify.Create) {
				m := matches[0]
				if fi, err := m.sh.root.Lstat(rootName(m.rel)); err == nil && fi.IsDir() {
					s.addRecursive(w, ev.Name)
				}
			}
			for _, m := range matches {
				record(m.sh, m.rel)
			}
		case name := <-flushc:
			p := pend[name]
			if p == nil {
				continue
			}
			if d := time.Until(p.deadline); d > 0 { // stale timer fire
				p.timer.Reset(d)
				continue
			}
			delete(pend, name)
			if s.lookupShare(name) != nil { // not removed meanwhile
				s.emit(name, p.paths)
			}
		}
	}
}

func (s *Server) emit(shareName string, set map[string]struct{}) {
	paths := make([]string, 0, len(set))
	for p := range set {
		paths = append(paths, p)
	}
	paths = reducePaths(paths, maxEventPaths)
	b, err := json.Marshal(changeEvent{Share: shareName, Paths: paths})
	if err != nil {
		return
	}
	msg := make([]byte, 0, len(b)+32)
	msg = append(msg, "event: change\ndata: "...)
	msg = append(msg, b...)
	msg = append(msg, "\n\n"...)
	s.log.Debug("changements détectés", "share", shareName, "paths", len(paths))
	s.hub.publish(msg)
}

// reducePaths sorts and dedupes paths, drops those under another listed path
// and, while more than limit remain, replaces them by their parent
// directories ("" being the share root).
func reducePaths(paths []string, limit int) []string {
	for {
		sort.Strings(paths)
		out := paths[:0]
		for _, p := range paths {
			if len(out) > 0 {
				last := out[len(out)-1]
				if p == last || last == "" || strings.HasPrefix(p, last+"/") {
					continue
				}
			}
			out = append(out, p)
		}
		paths = out
		if len(paths) <= limit {
			return paths
		}
		for i, p := range paths {
			d := path.Dir(p)
			if d == "." {
				d = ""
			}
			paths[i] = d
		}
	}
}

type shareMatch struct {
	sh  *share
	rel string
}

// sharesFor maps an absolute watched path to the shares containing it
// (several when shares overlap) and the path relative to each.
func (s *Server) sharesFor(abs string) []shareMatch {
	var out []shareMatch
	for _, sh := range s.activeShares() {
		if abs == sh.path {
			out = append(out, shareMatch{sh, ""})
		} else if within(abs, sh.path) {
			out = append(out, shareMatch{sh, filepath.ToSlash(abs[len(sh.path)+1:])})
		}
	}
	return out
}

// within reports whether p lies strictly below dir.
func within(p, dir string) bool {
	return strings.HasPrefix(p, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// watchShare starts watching a newly added share (no-op when the watcher
// is not running; it picks the share up when it starts).
func (s *Server) watchShare(sh *share) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watcher != nil {
		s.addRecursive(s.watcher, sh.path)
	}
}

// unwatchShare stops watching the directories of a removed share that no
// remaining share covers.
func (s *Server) unwatchShare(sh *share) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watcher == nil {
		return
	}
	remaining := s.activeShares()
	for _, p := range s.watcher.WatchList() {
		if p != sh.path && !within(p, sh.path) {
			continue
		}
		covered := false
		for _, o := range remaining {
			if p == o.path || within(p, o.path) {
				covered = true
				break
			}
		}
		if !covered {
			_ = s.watcher.Remove(p)
		}
	}
}

// addRecursive watches dir and all directories below it (symlinks are not
// followed).
func (s *Server) addRecursive(w *fsnotify.Watcher, dir string) {
	warned := false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && p != dir {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if err := w.Add(p); err != nil {
			if !warned {
				warned = true
				s.log.Warn("impossible de surveiller un dossier (limite inotify ?)", "path", p, "err", err)
			}
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipDir
			}
		}
		return nil
	})
}
