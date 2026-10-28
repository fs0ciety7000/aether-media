package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	apiPrefix     = "/agent/v1"
	tokenFileName = "agent.token"
)

type share struct {
	name string
	path string // absolute, symlinks resolved
	root *os.Root
}

// Server is an Aether agent. Create it with New.
type Server struct {
	cfg      Config
	version  string
	hostname string
	log      *slog.Logger
	token    string
	tokenSum [sha256.Size]byte

	shares     map[string]*share
	shareOrder []*share

	ffprobePath string // "" when unavailable
	probeSem    chan struct{}

	limiter *failLimiter
	hub     *hub

	// test hooks
	debounce   time.Duration
	pingEvery  time.Duration
	walkFlushN int

	handler http.Handler
	closeMu sync.Mutex
	closed  bool
}

// New validates cfg, opens every share with os.Root, loads or creates the
// pairing token in cfg.DataDir and detects ffprobe.
func New(cfg Config, version string) (*Server, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	if cfg.DataDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("agent: dossier de données introuvable : %w", err)
		}
		cfg.DataDir = filepath.Join(dir, "aether-agent")
	}
	s := &Server{
		cfg:        cfg,
		version:    version,
		log:        log,
		shares:     make(map[string]*share, len(cfg.Shares)),
		probeSem:   make(chan struct{}, 2),
		limiter:    newFailLimiter(5, time.Minute),
		hub:        newHub(),
		debounce:   10 * time.Second,
		pingEvery:  25 * time.Second,
		walkFlushN: 500,
	}
	s.hostname, _ = os.Hostname()
	for _, sc := range cfg.Shares {
		sh, err := openShare(sc)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.shares[sh.name] = sh
		s.shareOrder = append(s.shareOrder, sh)
	}
	tok, err := loadOrCreateToken(cfg.DataDir)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.token = tok
	s.tokenSum = sha256.Sum256([]byte(tok))
	s.ffprobePath = detectFFprobe(cfg.FFprobe)
	if s.ffprobePath == "" {
		log.Warn("ffprobe introuvable : /probe sera indisponible", "ffprobe", cfg.FFprobe)
	}
	s.handler = s.routes()
	return s, nil
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

func loadOrCreateToken(dir string) (string, error) {
	p := filepath.Join(dir, tokenFileName)
	b, err := os.ReadFile(p)
	switch {
	case err == nil:
		tok := strings.TrimSpace(string(b))
		if len(tok) < 32 {
			return "", fmt.Errorf("agent: jeton invalide dans %s (supprimez le fichier pour en générer un nouveau)", p)
		}
		if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(p, 0o600); err != nil {
				return "", fmt.Errorf("agent: permissions de %s : %w", p, err)
			}
		}
		return tok, nil
	case !errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("agent: lecture du jeton : %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("agent: création de %s : %w", dir, err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("agent: écriture du jeton : %w", err)
	}
	if _, err := io.WriteString(f, tok+"\n"); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("agent: écriture du jeton : %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("agent: écriture du jeton : %w", err)
	}
	return tok, nil
}

// Token returns the pairing token. Never log it.
func (s *Server) Token() string { return s.token }

// Handler returns the HTTP handler serving the agent protocol.
func (s *Server) Handler() http.Handler { return s.handler }

// Close releases the shares' os.Root handles. Run calls it on return.
func (s *Server) Close() {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for _, sh := range s.shareOrder {
		_ = sh.root.Close()
	}
}

// Run listens on cfg.Listen (TLS when configured), starts the file watcher
// and serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	defer s.Close()
	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return fmt.Errorf("agent: écoute sur %s : %w", s.cfg.Listen, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		if err := s.watch(ctx); err != nil {
			s.log.Warn("surveillance des fichiers désactivée", "err", err)
		}
		<-ctx.Done()
		s.hub.close() // ends the SSE streams so Shutdown can complete
	}()

	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelDebug),
	}
	scheme := "http"
	if s.cfg.TLSCert != "" {
		scheme = "https"
	}
	names := make([]string, len(s.shareOrder))
	for i, sh := range s.shareOrder {
		names[i] = sh.name
	}
	s.log.Info("agent Aether démarré", "adresse", scheme+"://"+ln.Addr().String(), "partages", strings.Join(names, ","), "ffprobe", s.ffprobePath != "")
	s.log.Info("Jeton d'appairage : " + s.token)

	errc := make(chan error, 1)
	go func() {
		if s.cfg.TLSCert != "" {
			errc <- srv.ServeTLS(ln, s.cfg.TLSCert, s.cfg.TLSKey)
		} else {
			errc <- srv.Serve(ln)
		}
	}()

	select {
	case err := <-errc:
		cancel()
		<-watchDone
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	s.log.Info("arrêt de l'agent")
	// Long-lived SSE streams end when the hub closes (watcher stopped).
	<-watchDone
	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close()
	}
	return nil
}

// ---- routing, auth, errors ----

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+apiPrefix+"/info", s.handleInfo)
	mux.HandleFunc("GET "+apiPrefix+"/walk", s.handleWalk)
	mux.HandleFunc("GET "+apiPrefix+"/stat", s.handleStat)
	mux.HandleFunc("GET "+apiPrefix+"/readdir", s.handleReadDir)
	mux.HandleFunc("GET "+apiPrefix+"/file", s.handleFile) // GET pattern also matches HEAD
	mux.HandleFunc("POST "+apiPrefix+"/probe", s.handleProbe)
	mux.HandleFunc("GET "+apiPrefix+"/events", s.handleEvents)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "point d'accès inconnu")
	})
	return s.authenticate(mux)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	scheme, tok, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", true // header present but malformed: counts as a failure
	}
	return strings.TrimSpace(tok), true
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ip := clientIP(r)
		if s.limiter.blocked(ip) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "trop d'échecs d'authentification")
			return
		}
		tok, present := bearer(r)
		if !present && r.URL.Path == apiPrefix+"/file" {
			tok = r.URL.Query().Get("t")
		}
		sum := sha256.Sum256([]byte(tok))
		if tok == "" || subtle.ConstantTimeCompare(sum[:], s.tokenSum[:]) != 1 {
			if s.limiter.fail(ip) {
				s.log.Warn("authentification refusée, adresse temporairement bloquée", "ip", ip)
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="aether-agent"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "jeton manquant ou invalide")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	h := w.Header()
	h.Del("Content-Length")
	h.Del("Last-Modified")
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{Error: code, Message: msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// ---- /info ----

type infoShare struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type infoResponse struct {
	Version  string      `json:"version"`
	Hostname string      `json:"hostname"`
	Shares   []infoShare `json:"shares"`
	FFprobe  bool        `json:"ffprobe"`
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	resp := infoResponse{
		Version:  s.version,
		Hostname: s.hostname,
		Shares:   make([]infoShare, 0, len(s.shareOrder)),
		FFprobe:  s.ffprobePath != "",
	}
	for _, sh := range s.shareOrder {
		is := infoShare{Name: sh.name}
		if s.cfg.ExposePaths {
			is.Path = sh.path
		}
		resp.Shares = append(resp.Shares, is)
	}
	writeJSON(w, resp)
}

// ---- failed-auth rate limiter ----

type failWindow struct {
	start time.Time
	count int
}

type failLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	m      map[string]*failWindow
	now    func() time.Time
}

const limiterMaxEntries = 4096

func newFailLimiter(maxFails int, window time.Duration) *failLimiter {
	return &failLimiter{max: maxFails, window: window, m: make(map[string]*failWindow), now: time.Now}
}

// blocked reports whether ip exhausted its failure budget for the window.
func (l *failLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	fw := l.m[ip]
	if fw == nil {
		return false
	}
	if l.now().Sub(fw.start) >= l.window {
		delete(l.m, ip)
		return false
	}
	return fw.count >= l.max
}

// fail records a failure; it reports whether ip just became blocked.
func (l *failLimiter) fail(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	fw := l.m[ip]
	if fw == nil || now.Sub(fw.start) >= l.window {
		if len(l.m) >= limiterMaxEntries {
			l.cleanupLocked(now)
		}
		fw = &failWindow{start: now}
		l.m[ip] = fw
	}
	fw.count++
	return fw.count == l.max
}

func (l *failLimiter) cleanupLocked(now time.Time) {
	for k, v := range l.m {
		if now.Sub(v.start) >= l.window {
			delete(l.m, k)
		}
	}
	if len(l.m) >= limiterMaxEntries {
		// Still full (address flood): evict arbitrary entries. Blocked
		// addresses may get a fresh budget, which only costs a few guesses
		// against a 256-bit token.
		n := len(l.m) / 2
		for k := range l.m {
			if n == 0 {
				break
			}
			delete(l.m, k)
			n--
		}
	}
}
