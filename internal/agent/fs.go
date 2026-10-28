package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// target resolves the share and path query parameters. On failure it writes
// the error response and returns ok=false.
func (s *Server) target(w http.ResponseWriter, r *http.Request) (sh *share, rel string, ok bool) {
	q := r.URL.Query()
	name := q.Get("share")
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_share", "paramètre share manquant")
		return nil, "", false
	}
	sh = s.lookupShare(name)
	if sh == nil {
		writeError(w, http.StatusNotFound, "not_found", "partage inconnu")
		return nil, "", false
	}
	rel, err := vfs.CleanRel(q.Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_path", "chemin invalide")
		return nil, "", false
	}
	if hiddenPath(rel) {
		writeError(w, http.StatusNotFound, "not_found", "fichier introuvable")
		return nil, "", false
	}
	return sh, rel, true
}

// hiddenPath reports whether a path goes through a hidden file or folder.
// Hidden entries (".ssh", ".config"...) are never media: they are neither
// listed nor served, even when a broad folder such as a home directory is
// shared.
func hiddenPath(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// rootName converts a cleaned relative path ("" = root) to an os.Root name.
func rootName(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}

func isEscape(err error) bool {
	// os.Root does not export its "path escapes from parent" error.
	return err != nil && strings.Contains(err.Error(), "path escapes from parent")
}

// fsError maps a filesystem error to an HTTP response. Escaping paths are
// reported as not found so as not to reveal anything outside the share.
func (s *Server) fsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrClosed):
		writeError(w, http.StatusNotFound, "not_found", "partage supprimé")
	case errors.Is(err, fs.ErrNotExist), isEscape(err):
		writeError(w, http.StatusNotFound, "not_found", "introuvable")
	case errors.Is(err, fs.ErrPermission):
		writeError(w, http.StatusForbidden, "forbidden", "accès refusé")
	default:
		s.log.Warn("erreur de fichier", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "erreur interne")
	}
}

func makeEntry(rel string, fi fs.FileInfo) vfs.Entry {
	e := vfs.Entry{Path: rel, ModTime: fi.ModTime().UTC(), IsDir: fi.IsDir()}
	if !e.IsDir {
		e.Size = fi.Size()
	}
	return e
}

// dirEntry builds the Entry for a child returned by ReadDir. Symbolic links
// are followed inside the share (os.Root refuses escaping targets); links to
// directories are reported but never descended into (see walk), and broken
// or escaping links are skipped (ok=false). Other special files are skipped.
func dirEntry(sh *share, rel string, d fs.DirEntry) (vfs.Entry, bool) {
	t := d.Type()
	switch {
	case t&fs.ModeSymlink != 0:
		fi, err := sh.root.Stat(rel)
		if err != nil || (!fi.Mode().IsRegular() && !fi.IsDir()) {
			return vfs.Entry{}, false
		}
		return makeEntry(rel, fi), true
	case t.IsDir() || t.IsRegular():
		fi, err := d.Info()
		if err != nil {
			return vfs.Entry{}, false
		}
		return makeEntry(rel, fi), true
	}
	return vfs.Entry{}, false
}

func joinRel(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// ---- /stat ----

func (s *Server) handleStat(w http.ResponseWriter, r *http.Request) {
	sh, rel, ok := s.target(w, r)
	if !ok {
		return
	}
	fi, err := sh.root.Stat(rootName(rel))
	if err != nil {
		s.fsError(w, err)
		return
	}
	writeJSON(w, makeEntry(rel, fi))
}

// ---- /readdir ----

func (s *Server) handleReadDir(w http.ResponseWriter, r *http.Request) {
	sh, rel, ok := s.target(w, r)
	if !ok {
		return
	}
	fi, err := sh.root.Stat(rootName(rel))
	if err != nil {
		s.fsError(w, err)
		return
	}
	if !fi.IsDir() {
		writeError(w, http.StatusBadRequest, "not_a_directory", "pas un dossier")
		return
	}
	des, err := fs.ReadDir(sh.root.FS(), rootName(rel))
	if err != nil {
		s.fsError(w, err)
		return
	}
	out := make([]vfs.Entry, 0, len(des))
	for _, d := range des {
		if strings.HasPrefix(d.Name(), ".") {
			continue
		}
		if e, ok := dirEntry(sh, joinRel(rel, d.Name()), d); ok {
			out = append(out, e)
		}
	}
	writeJSON(w, out)
}

// ---- /walk ----

type walkEnd struct {
	Done  bool   `json:"done,omitempty"`
	Error string `json:"error,omitempty"`
}

func (s *Server) handleWalk(w http.ResponseWriter, r *http.Request) {
	sh, rel, ok := s.target(w, r)
	if !ok {
		return
	}
	fi, err := sh.root.Stat(rootName(rel))
	if err != nil {
		s.fsError(w, err)
		return
	}
	if !fi.IsDir() {
		writeError(w, http.StatusBadRequest, "not_a_directory", "pas un dossier")
		return
	}
	des, err := fs.ReadDir(sh.root.FS(), rootName(rel))
	if err != nil {
		s.fsError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	bw := bufio.NewWriterSize(w, 64<<10)
	enc := json.NewEncoder(bw)
	ctx := r.Context()
	n := 0
	flush := func() error {
		if err := bw.Flush(); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		return nil
	}

	var walk func(dir string, des []fs.DirEntry) error
	walk = func(dir string, des []fs.DirEntry) error {
		for _, d := range des {
			if err := ctx.Err(); err != nil {
				return err
			}
			if strings.HasPrefix(d.Name(), ".") {
				continue // hidden: never exposed (see hiddenPath)
			}
			child := joinRel(dir, d.Name())
			e, ok := dirEntry(sh, child, d)
			if !ok {
				continue
			}
			if err := enc.Encode(e); err != nil {
				return err
			}
			n++
			if n%s.walkFlushN == 0 {
				if err := flush(); err != nil {
					return err
				}
			}
			if !d.IsDir() { // regular file, or symlink (never descended)
				continue
			}
			sub, err := fs.ReadDir(sh.root.FS(), child)
			if err != nil {
				s.log.Debug("dossier illisible ignoré", "share", sh.name, "path", child, "err", err)
				continue
			}
			if err := walk(child, sub); err != nil {
				return err
			}
		}
		return nil
	}
	start := time.Now()
	err = walk(rel, des)
	if ctx.Err() != nil {
		return // client gone: nothing more to say
	}
	if err == nil && sh.removed.Load() {
		// Unreadable directories were skipped because the share went away:
		// the listing is incomplete and must not be taken as complete.
		err = errShareRemoved
	}
	if err != nil {
		_ = enc.Encode(walkEnd{Error: err.Error()})
	} else {
		_ = enc.Encode(walkEnd{Done: true})
	}
	_ = flush()
	s.log.Debug("parcours terminé", "share", sh.name, "path", rel, "entries", n, "duration", time.Since(start))
}

var errShareRemoved = errors.New("partage supprimé")

// ---- /file ----

var contentTypes = map[string]string{
	".mkv": "video/x-matroska", ".mk3d": "video/x-matroska", ".mka": "audio/x-matroska",
	".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime",
	".webm": "video/webm", ".avi": "video/x-msvideo", ".wmv": "video/x-ms-wmv",
	".flv": "video/x-flv", ".ts": "video/mp2t", ".m2ts": "video/mp2t", ".mts": "video/mp2t",
	".mpg": "video/mpeg", ".mpeg": "video/mpeg", ".ogv": "video/ogg", ".3gp": "video/3gpp",
	".mp3": "audio/mpeg", ".flac": "audio/flac", ".m4a": "audio/mp4", ".m4b": "audio/mp4",
	".aac": "audio/aac", ".ogg": "audio/ogg", ".oga": "audio/ogg", ".opus": "audio/opus",
	".wav": "audio/wav", ".wma": "audio/x-ms-wma", ".alac": "audio/mp4",
	".srt": "application/x-subrip", ".vtt": "text/vtt", ".ass": "text/x-ssa", ".ssa": "text/x-ssa",
	".sub": "text/plain", ".idx": "text/plain", ".sup": "application/octet-stream",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp",
	".gif": "image/gif", ".tbn": "image/jpeg",
	".nfo": "text/xml; charset=utf-8", ".xml": "text/xml; charset=utf-8",
	".txt": "text/plain; charset=utf-8", ".json": "application/json",
}

func contentType(name string) string {
	if ct, ok := contentTypes[strings.ToLower(path.Ext(name))]; ok {
		return ct
	}
	return "application/octet-stream"
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	sh, rel, ok := s.target(w, r)
	if !ok {
		return
	}
	if rel == "" {
		writeError(w, http.StatusBadRequest, "not_a_file", "pas un fichier")
		return
	}
	f, err := sh.root.Open(rel)
	if err != nil {
		s.fsError(w, err)
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		s.fsError(w, err)
		return
	}
	if !fi.Mode().IsRegular() {
		writeError(w, http.StatusBadRequest, "not_a_file", "pas un fichier")
		return
	}
	w.Header().Set("Content-Type", contentType(rel))
	w.Header().Set("Cache-Control", "private, no-transform")
	http.ServeContent(w, r, path.Base(rel), fi.ModTime(), f)
}
