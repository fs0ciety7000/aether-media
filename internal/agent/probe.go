package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/toolpath"
)

const (
	probeTimeout   = 60 * time.Second
	probeMaxOutput = 32 << 20
	stderrTail     = 2048
)

// detectFFprobe returns the resolved ffprobe path, or "" when it is missing
// or does not run.
func detectFFprobe(name string) string {
	p, err := exec.LookPath(toolpath.Resolve(name))
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, p, "-version").Run(); err != nil {
		return ""
	}
	return p
}

// limitedBuffer keeps at most max bytes and drops the rest.
type limitedBuffer struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); len(p) > room {
		b.overflow = true
		if room > 0 {
			b.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

// tailBuffer keeps the last max bytes written.
type tailBuffer struct {
	buf []byte
	max int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append(b.buf[:0], b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

// localPath resolves rel to an absolute path for an external program. os.Root
// does not hand out paths, so the file is first checked through the root
// (exists, regular, no escaping link), then the joined path is resolved with
// EvalSymlinks and must still lie inside the share.
func (sh *share) localPath(rel string) (string, error) {
	fi, err := sh.root.Stat(rel)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", errNotRegular
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(sh.path, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(resolved, sh.path+string(filepath.Separator)) {
		return "", errEscape
	}
	return resolved, nil
}

var (
	errNotRegular = errors.New("not a regular file")
	errEscape     = errors.New("path escapes from parent")
)

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	sh, rel, ok := s.target(w, r)
	if !ok {
		return
	}
	if s.ffprobePath == "" {
		writeError(w, http.StatusInternalServerError, "ffprobe_unavailable", "ffprobe n'est pas installé sur l'agent")
		return
	}
	if rel == "" {
		writeError(w, http.StatusBadRequest, "not_a_file", "pas un fichier")
		return
	}
	local, err := sh.localPath(rel)
	switch {
	case errors.Is(err, errNotRegular):
		writeError(w, http.StatusBadRequest, "not_a_file", "pas un fichier")
		return
	case err != nil:
		s.fsError(w, err)
		return
	}

	ctx := r.Context()
	select {
	case s.probeSem <- struct{}{}:
		defer func() { <-s.probeSem }()
	case <-ctx.Done():
		return
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	stdout := &limitedBuffer{max: probeMaxOutput}
	stderr := &tailBuffer{max: stderrTail}
	cmd := exec.CommandContext(ctx, s.ffprobePath,
		"-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters",
		"file:"+local)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 5 * time.Second
	start := time.Now()
	runErr := cmd.Run()
	if r.Context().Err() != nil {
		return
	}
	out := stdout.buf.Bytes()
	if runErr != nil || stdout.overflow || !json.Valid(out) {
		msg := strings.TrimSpace(string(stderr.buf))
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			msg = "délai dépassé (60 s)"
		case msg == "" && runErr != nil:
			msg = runErr.Error()
		case msg == "":
			msg = "sortie ffprobe invalide"
		}
		s.log.Debug("analyse ffprobe échouée", "share", sh.name, "path", rel, "err", runErr)
		writeError(w, http.StatusUnprocessableEntity, "probe_failed", msg)
		return
	}
	s.log.Debug("analyse ffprobe", "share", sh.name, "path", rel, "duration", time.Since(start))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}
