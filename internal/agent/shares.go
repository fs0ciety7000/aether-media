package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/fs0ciety7000/aether-media/internal/fsbrowse"
)

const (
	maxManagedShares = 256
	maxBodyBytes     = 8 << 10
)

type share struct {
	name    string
	path    string // absolute, symlinks resolved
	root    *os.Root
	managed bool        // added from the UI (agent-state.json), deletable
	removed atomic.Bool // set once deleted: in-flight requests must not report success
}

func openShare(sc Share) (*share, error) {
	abs, err := filepath.Abs(sc.Path)
	if err != nil {
		return nil, fmt.Errorf("agent: partage %q : %w", sc.Name, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("agent: partage %q : %w", sc.Name, err)
	}
	fi, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("agent: partage %q : %w", sc.Name, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("agent: partage %q : %s n'est pas un dossier", sc.Name, resolved)
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, fmt.Errorf("agent: partage %q : %w", sc.Name, err)
	}
	return &share{name: sc.Name, path: resolved, root: root}, nil
}

// lookupShare returns the active share called name, or nil.
func (s *Server) lookupShare(name string) *share {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shares[name]
}

// activeShares returns a snapshot of the served shares, in order.
func (s *Server) activeShares() []*share {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]*share(nil), s.shareOrder...)
}

func (s *Server) activateLocked(sh *share) {
	s.shares[sh.name] = sh
	s.shareOrder = append(s.shareOrder, sh)
}

func (s *Server) deactivateLocked(sh *share) {
	delete(s.shares, sh.name)
	for i, o := range s.shareOrder {
		if o == sh {
			s.shareOrder = append(s.shareOrder[:i:i], s.shareOrder[i+1:]...)
			break
		}
	}
}

// statusError is an error carrying its HTTP response.
type statusError struct {
	status int
	code   string
	msg    string
}

func (e *statusError) Error() string { return e.code + ": " + e.msg }

func newStatusError(status int, code, msg string) error {
	return &statusError{status: status, code: code, msg: msg}
}

func (s *Server) writeStatusError(w http.ResponseWriter, err error) {
	var se *statusError
	if errors.As(err, &se) {
		writeError(w, se.status, se.code, se.msg)
		return
	}
	s.log.Warn("erreur interne", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "erreur interne")
}

// accentFold maps common accented Latin letters (lower case) to ASCII.
var accentFold = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'æ': "ae",
	'ç': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ñ': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'œ': "oe",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y", 'ß': "ss",
}

const derivedNameMax = 32 // leaves room for a "-NNN" suffix within 40

// deriveShareName turns a folder name into a share name ([a-z0-9_-]):
// lower case, accents folded, other characters replaced by '-'.
func deriveShareName(base string) string {
	var b strings.Builder
	dash := false
	put := func(c rune) {
		if c == '-' {
			if dash || b.Len() == 0 {
				return
			}
			dash = true
		} else {
			dash = false
		}
		b.WriteRune(c)
	}
	for _, r := range strings.ToLower(base) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			put(r)
		case accentFold[r] != "":
			for _, c := range accentFold[r] {
				put(c)
			}
		default:
			put('-')
		}
	}
	n := b.String()
	if len(n) > derivedNameMax {
		n = n[:derivedNameMax]
	}
	n = strings.Trim(n, "-")
	if n == "" {
		n = "partage"
	}
	return n
}

// uniqueName appends "-2", "-3"… to base until it is not taken.
func uniqueName(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for i := 2; ; i++ {
		n := base + "-" + strconv.Itoa(i)
		if !taken(n) {
			return n
		}
	}
}

// nameTakenLocked reports whether a share (served, or managed but currently
// unavailable) already uses name.
func (s *Server) nameTakenLocked(name string) bool {
	if s.shares[name] != nil {
		return true
	}
	for _, sc := range s.state.Shares {
		if sc.Name == name {
			return true
		}
	}
	return false
}

// AddShare adds a managed share for the directory dir (absolute, within the
// browse roots). name may be empty (derived from the folder). It returns the
// existing share when one already serves dir (created=false). The share is
// persisted, served and watched immediately.
func (s *Server) AddShare(dir, name string) (Share, bool, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return Share{}, false, newStatusError(http.StatusBadRequest, "invalid_path", "chemin absolu attendu")
	}
	// An already shared folder is returned as is, even when it lies outside
	// the browse roots (agent.toml shares); GET /shares lists them anyway.
	if resolved, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
		s.mu.RLock()
		for _, sh := range s.shareOrder {
			if sh.path == resolved {
				s.mu.RUnlock()
				return Share{Name: sh.name, Path: sh.path}, false, nil
			}
		}
		s.mu.RUnlock()
	}
	resolved, ok := fsbrowse.Allowed(s.browseRoots, dir)
	if !ok {
		return Share{}, false, newStatusError(http.StatusForbidden, "denied", "dossier inexistant ou hors des emplacements autorisés")
	}
	fi, err := os.Stat(resolved)
	if err != nil {
		return Share{}, false, newStatusError(http.StatusNotFound, "not_found", "dossier introuvable")
	}
	if !fi.IsDir() {
		return Share{}, false, newStatusError(http.StatusBadRequest, "not_a_directory", "pas un dossier")
	}
	if name != "" && !validShareName(name) {
		return Share{}, false, newStatusError(http.StatusBadRequest, "invalid_name", "nom invalide (attendu [a-z0-9_-]{1,40})")
	}

	s.mu.Lock()
	for _, sh := range s.shareOrder {
		if sh.path == resolved {
			s.mu.Unlock()
			return Share{Name: sh.name, Path: sh.path}, false, nil
		}
	}
	// A managed share that was unavailable at startup (unmounted disk…)
	// comes back under its name.
	reactivate := -1
	for i, sc := range s.state.Shares {
		if sc.Path == resolved && s.shares[sc.Name] == nil {
			reactivate = i
			name = sc.Name
			break
		}
	}
	if reactivate < 0 {
		switch {
		case name != "" && s.nameTakenLocked(name):
			s.mu.Unlock()
			return Share{}, false, newStatusError(http.StatusConflict, "name_taken", "un partage porte déjà ce nom")
		case name == "":
			name = uniqueName(deriveShareName(filepath.Base(resolved)), s.nameTakenLocked)
		}
		if len(s.state.Shares) >= maxManagedShares {
			s.mu.Unlock()
			return Share{}, false, newStatusError(http.StatusConflict, "too_many_shares", "trop de partages")
		}
	}
	sh, err := openShare(Share{Name: name, Path: resolved})
	if err != nil {
		s.mu.Unlock()
		return Share{}, false, err
	}
	sh.managed = true
	if reactivate < 0 {
		s.state.Shares = append(s.state.Shares, Share{Name: sh.name, Path: sh.path})
		if err := s.saveLocked(); err != nil {
			s.state.Shares = s.state.Shares[:len(s.state.Shares)-1]
			s.mu.Unlock()
			_ = sh.root.Close()
			return Share{}, false, err
		}
	}
	s.activateLocked(sh)
	s.mu.Unlock()

	s.watchShare(sh)
	s.log.Info("partage ajouté", "share", sh.name, "path", sh.path)
	return Share{Name: sh.name, Path: sh.path}, true, nil
}

// RemoveShare deletes a managed share. Shares from agent.toml cannot be
// removed (409). In-flight requests on the share fail cleanly.
func (s *Server) RemoveShare(name string) error {
	s.mu.Lock()
	sh := s.shares[name]
	if sh != nil && !sh.managed {
		s.mu.Unlock()
		return newStatusError(http.StatusConflict, "not_managed", "partage défini dans agent.toml : modifiez le fichier")
	}
	idx := -1
	for i, sc := range s.state.Shares {
		if sc.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return newStatusError(http.StatusNotFound, "not_found", "partage inconnu")
	}
	prev := s.state.Shares
	s.state.Shares = append(append([]Share(nil), prev[:idx]...), prev[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		s.state.Shares = prev
		s.mu.Unlock()
		return err
	}
	if sh != nil {
		s.deactivateLocked(sh)
		sh.removed.Store(true)
	}
	s.mu.Unlock()

	if sh != nil {
		s.unwatchShare(sh)
		_ = sh.root.Close() // the descriptor lives until in-flight operations end
	}
	s.log.Info("partage supprimé", "share", name)
	return nil
}

// ---- HTTP: /shares ----

type shareInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Managed     bool   `json:"managed"`
	Unavailable bool   `json:"unavailable,omitempty"` // managed share whose folder could not be opened
}

func (s *Server) shareInfos() []shareInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]shareInfo, 0, len(s.shareOrder)+len(s.state.Shares))
	for _, sh := range s.shareOrder {
		out = append(out, shareInfo{Name: sh.name, Path: sh.path, Managed: sh.managed})
	}
	for _, sc := range s.state.Shares {
		if s.shares[sc.Name] == nil {
			out = append(out, shareInfo{Name: sc.Name, Path: sc.Path, Managed: true, Unavailable: true})
		}
	}
	return out
}

func (s *Server) handleListShares(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.shareInfos())
}

// decodeBody decodes a small JSON request body.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "corps JSON invalide")
		return false
	}
	return true
}

func (s *Server) handleAddShare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	sc, created, err := s.AddShare(req.Path, strings.TrimSpace(req.Name))
	if err != nil {
		s.writeStatusError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if created {
		w.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(w).Encode(sc)
}

func (s *Server) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	if err := s.RemoveShare(r.PathValue("name")); err != nil {
		s.writeStatusError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- HTTP: /browse ----

type browseResponse struct {
	Path   string         `json:"path"`
	Parent string         `json:"parent,omitempty"`
	Dirs   []fsbrowse.Dir `json:"dirs"`
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		dirs, _ := fsbrowse.List(s.browseRoots, "")
		writeJSON(w, browseResponse{Dirs: dirs})
		return
	}
	if !filepath.IsAbs(p) || strings.ContainsRune(p, 0) {
		writeError(w, http.StatusBadRequest, "invalid_path", "chemin absolu attendu")
		return
	}
	resolved, ok := fsbrowse.Allowed(s.browseRoots, p)
	if !ok {
		writeError(w, http.StatusForbidden, "denied", "dossier inexistant ou hors des emplacements autorisés")
		return
	}
	if fi, err := os.Stat(resolved); err != nil || !fi.IsDir() {
		writeError(w, http.StatusBadRequest, "not_a_directory", "pas un dossier")
		return
	}
	dirs, err := fsbrowse.List(s.browseRoots, resolved)
	switch {
	case errors.Is(err, fsbrowse.ErrDenied):
		writeError(w, http.StatusForbidden, "denied", "hors des emplacements autorisés")
		return
	case errors.Is(err, fs.ErrPermission):
		writeError(w, http.StatusForbidden, "forbidden", "accès refusé")
		return
	case err != nil:
		s.log.Warn("navigation impossible", "path", resolved, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}
	resp := browseResponse{Path: resolved, Dirs: dirs}
	if d := filepath.Dir(resolved); d != resolved {
		if pr, ok := fsbrowse.Allowed(s.browseRoots, d); ok {
			resp.Parent = pr
		}
	}
	writeJSON(w, resp)
}
