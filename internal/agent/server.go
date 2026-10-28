package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agentproto"
	"github.com/fs0ciety7000/aether-media/internal/fsbrowse"
	"github.com/fsnotify/fsnotify"
)

const (
	apiPrefix     = "/agent/v1"
	tokenFileName = "agent.token"
)

// Server is an Aether agent. Create it with New.
type Server struct {
	cfg      Config
	version  string
	hostname string
	uid      string
	name     string
	log      *slog.Logger
	token    string
	tokenSum [sha256.Size]byte

	// mu guards shares, shareOrder, state and serverSums.
	mu         sync.RWMutex
	shares     map[string]*share
	shareOrder []*share
	state      agentState
	serverSums []serverSum

	browseRoots []string

	ffprobePath string // "" when unavailable
	probeSem    chan struct{}

	limiter     *failLimiter // bad tokens
	pairLimiter *failLimiter // bad pairing codes
	pairing     *pairing
	pairKDF     chan struct{} // one Argon2id derivation at a time

	// tls is the served certificate (nil with tls = false).
	tls         *agentTLS
	plainWarned sync.Map // caller key → struct{}: plain HTTP use logged once
	hub         *hub
	httpPort    atomic.Int32 // actual listening port once Run started

	watchMu sync.Mutex
	watcher *fsnotify.Watcher // nil while the watcher is not running

	// test hooks
	debounce   time.Duration
	pingEvery  time.Duration
	walkFlushN int

	handler http.Handler
	closeMu sync.Mutex
	closed  bool
}

// New validates cfg, loads agent-state.json, opens every share (agent.toml
// and managed ones) with os.Root, loads or creates the legacy token in
// cfg.DataDir and detects ffprobe. An empty configuration is valid: the
// agent then waits to be paired and configured from the server's UI.
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
		cfg:         cfg,
		version:     version,
		log:         log,
		shares:      make(map[string]*share, len(cfg.Shares)),
		probeSem:    make(chan struct{}, 2),
		limiter:     newFailLimiter(5, time.Minute),
		pairLimiter: newFailLimiter(pairMaxFails, pairFailSpan),
		pairing:     newPairing(log),
		pairKDF:     make(chan struct{}, 1),
		hub:         newHub(),
		debounce:    10 * time.Second,
		pingEvery:   25 * time.Second,
		walkFlushN:  500,
	}
	s.hostname, _ = os.Hostname()
	s.browseRoots = cfg.BrowseRoots
	if len(s.browseRoots) == 0 {
		s.browseRoots = fsbrowse.DefaultRoots()
	}
	for _, sc := range cfg.Shares {
		sh, err := openShare(sc)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.activateLocked(sh)
	}
	tok, tokCreated, err := loadOrCreateToken(cfg.DataDir)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.token = tok
	s.tokenSum = sha256.Sum256([]byte(tok))
	if err := s.loadState(tokCreated); err != nil {
		s.Close()
		return nil, err
	}
	if !cfg.DisableTLS {
		t, created, err := loadTLS(cfg, s.hostname)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.tls = t
		if created {
			log.Info("certificat TLS auto-signé créé", "fichier", filepath.Join(cfg.DataDir, certFileName))
		}
	}
	s.ffprobePath = detectFFprobe(cfg.FFprobe)
	if s.ffprobePath == "" {
		log.Warn("ffprobe introuvable : /probe sera indisponible", "ffprobe", cfg.FFprobe)
	}
	s.handler = s.routes()
	return s, nil
}

// loadState reads agent-state.json, opens the managed shares and saves the
// state back when it was created, upgraded or cleaned up. tokenCreated
// tells whether agent.token was just generated.
func (s *Server) loadState(tokenCreated bool) error {
	st, dirty, err := loadState(s.cfg.DataDir)
	if err != nil {
		return err
	}
	if st.Version < stateVersion {
		if st.Version == 0 {
			// Written by an agent that only spoke HTTP (or none yet, but an
			// agent.token from a v1 agent): what was paired keeps working
			// over HTTP until it pairs again.
			for i := range st.Servers {
				st.Servers[i].Plain = true
			}
			st.LegacyTokenPlain = !tokenCreated
		}
		st.Version = stateVersion
		dirty = true
	}
	if tokenCreated && st.LegacyTokenPlain {
		st.LegacyTokenPlain = false
		dirty = true
	}
	if st.Name == "" {
		st.Name = s.hostname
		dirty = true
	}
	if st.Name == "" {
		st.Name = "Agent Aether"
	}
	s.state = st
	s.uid = st.UID
	s.name = cleanDisplayName(st.Name, maxNameLen)
	if s.cfg.Name != "" {
		s.name = cleanDisplayName(s.cfg.Name, maxNameLen)
	}
	kept := make([]Share, 0, len(st.Shares))
	for _, sc := range st.Shares {
		if !validShareName(sc.Name) || sc.Path == "" {
			s.log.Warn("partage géré invalide ignoré", "share", sc.Name)
			dirty = true
			continue
		}
		if dup := s.nameOrPathUsed(sc, kept); dup != "" {
			s.log.Warn("partage géré ignoré : "+dup, "share", sc.Name, "path", sc.Path)
			dirty = true
			continue
		}
		kept = append(kept, sc)
		sh, err := openShare(sc)
		if err != nil {
			// Unmounted disk…: keep it in the state, it comes back on restart.
			s.log.Warn("partage indisponible", "share", sc.Name, "err", err)
			continue
		}
		sh.managed = true
		s.activateLocked(sh)
	}
	s.state.Shares = kept
	s.rebuildSumsLocked()
	if dirty {
		if err := s.saveLocked(); err != nil {
			return err
		}
	}
	return nil
}

// nameOrPathUsed reports why sc collides with an agent.toml share or an
// earlier managed one ("" when it does not).
func (s *Server) nameOrPathUsed(sc Share, kept []Share) string {
	for _, o := range kept {
		if o.Name == sc.Name {
			return "nom en double"
		}
	}
	for _, sh := range s.shareOrder {
		if sh.name == sc.Name {
			return "nom déjà utilisé par agent.toml"
		}
		if sh.path == sc.Path {
			return "dossier déjà partagé par agent.toml"
		}
	}
	return ""
}

func loadOrCreateToken(dir string) (tok string, created bool, err error) {
	p := filepath.Join(dir, tokenFileName)
	b, err := os.ReadFile(p)
	switch {
	case err == nil:
		tok := strings.TrimSpace(string(b))
		if len(tok) < 32 {
			return "", false, fmt.Errorf("agent: jeton invalide dans %s (supprimez le fichier pour en générer un nouveau)", p)
		}
		if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(p, 0o600); err != nil {
				return "", false, fmt.Errorf("agent: permissions de %s : %w", p, err)
			}
		}
		return tok, false, nil
	case !errors.Is(err, os.ErrNotExist):
		return "", false, fmt.Errorf("agent: lecture du jeton : %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("agent: création de %s : %w", dir, err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", false, err
	}
	tok = base64.RawURLEncoding.EncodeToString(raw)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", false, fmt.Errorf("agent: écriture du jeton : %w", err)
	}
	if _, err := io.WriteString(f, tok+"\n"); err != nil {
		_ = f.Close()
		return "", false, fmt.Errorf("agent: écriture du jeton : %w", err)
	}
	if err := f.Close(); err != nil {
		return "", false, fmt.Errorf("agent: écriture du jeton : %w", err)
	}
	return tok, true, nil
}

// Token returns the pairing token. Never log it.
func (s *Server) Token() string { return s.token }

// Handler returns the HTTP handler serving the agent protocol.
func (s *Server) Handler() http.Handler { return s.handler }

// Fingerprint returns the lowercase hex SHA-256 of the served certificate
// ("" with tls = false).
func (s *Server) Fingerprint() string {
	if s.tls == nil {
		return ""
	}
	return s.tls.fingerprint
}

// TLSConfig returns the TLS configuration the agent serves (nil with
// tls = false). Tests use it to serve the handler over HTTPS.
func (s *Server) TLSConfig() *tls.Config {
	if s.tls == nil {
		return nil
	}
	return s.tls.config()
}

// WrapListener returns the listener Run serves on: HTTPS and plain HTTP on
// the same port when TLS is on (plain HTTP is limited to the local page and
// pre-HTTPS pairings), ln itself with tls = false.
func (s *Server) WrapListener(ln net.Listener) net.Listener {
	if s.tls == nil {
		return ln
	}
	return newSniffListener(ln, s.tls.config())
}

func (s *Server) scheme() string {
	if s.tls != nil {
		return "https"
	}
	return "http"
}

// UID returns the agent's stable identifier.
func (s *Server) UID() string { return s.uid }

// Name returns the agent's display name.
func (s *Server) Name() string { return s.name }

// Close releases the shares' os.Root handles. Run calls it on return.
func (s *Server) Close() {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for _, sh := range s.activeShares() {
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
	if ta, ok := ln.Addr().(*net.TCPAddr); ok {
		s.httpPort.Store(int32(ta.Port))
	}
	var bg sync.WaitGroup
	defer bg.Wait() // runs after cancel below
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bg.Go(func() { s.pairing.rotateLoop(ctx) })
	if s.cfg.DiscoveryPort > 0 {
		pc, err := net.ListenPacket("udp", s.discoveryAddr())
		if err != nil {
			s.log.Warn("découverte réseau indisponible", "port", s.cfg.DiscoveryPort, "err", err)
		} else {
			s.log.Info("découverte réseau active", "udp", pc.LocalAddr().String())
			bg.Go(func() { s.serveDiscovery(ctx, pc) })
		}
	}

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
	scheme := s.scheme()
	shares := s.activeShares()
	names := make([]string, len(shares))
	for i, sh := range shares {
		names[i] = sh.name
	}
	s.log.Info("agent Aether démarré", "nom", s.name, "adresse", scheme+"://"+ln.Addr().String(), "partages", strings.Join(names, ","), "ffprobe", s.ffprobePath != "")
	if urls := s.listenURLs(); len(urls) > 0 {
		s.log.Info("page locale : " + scheme + "://localhost:" + strconv.Itoa(int(s.httpPort.Load())) + "/ · adresses : " + strings.Join(urls, " "))
	}
	s.log.Info("Jeton d'appairage manuel (agent.token) : " + s.token)
	s.logTLS()
	s.pairing.current() // logs the pairing code

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(s.WrapListener(ln)) }()

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
	mux.HandleFunc("DELETE "+apiPrefix+"/pair", s.handleUnpair)
	mux.HandleFunc("GET "+apiPrefix+"/browse", s.handleBrowse)
	mux.HandleFunc("GET "+apiPrefix+"/shares", s.handleListShares)
	mux.HandleFunc("POST "+apiPrefix+"/shares", s.handleAddShare)
	mux.HandleFunc("DELETE "+apiPrefix+"/shares/{name}", s.handleDeleteShare)
	mux.HandleFunc("GET "+apiPrefix+"/walk", s.handleWalk)
	mux.HandleFunc("GET "+apiPrefix+"/stat", s.handleStat)
	mux.HandleFunc("GET "+apiPrefix+"/readdir", s.handleReadDir)
	mux.HandleFunc("GET "+apiPrefix+"/file", s.handleFile) // GET pattern also matches HEAD
	mux.HandleFunc("POST "+apiPrefix+"/probe", s.handleProbe)
	mux.HandleFunc("GET "+apiPrefix+"/events", s.handleEvents)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "point d'accès inconnu")
	})

	// Unauthenticated routes: pairing, and the local status page/JSON
	// (loopback clients only). Everything else requires a token.
	public := http.NewServeMux()
	public.HandleFunc("POST "+apiPrefix+"/pair", s.handlePair)
	public.HandleFunc("GET /{$}", localOnly(s.handleLocalPage))
	public.HandleFunc("GET "+apiPrefix+"/local/status", localOnly(s.handleLocalStatus))
	public.Handle("/", s.authenticate(mux))
	return public
}

// logTLS logs the certificate fingerprint and the HTTP compatibility state.
func (s *Server) logTLS() {
	if s.tls == nil {
		s.log.Warn("HTTPS désactivé (tls = false) : à n'utiliser que derrière un proxy HTTPS ou un VPN")
		return
	}
	src := "auto-signé"
	if s.tls.source == certCustom {
		src = "tls_cert"
	}
	s.log.Info("Empreinte du certificat TLS (SHA-256, " + src + ") : " + agentproto.FormatFingerprint(s.tls.fingerprint))
	if s.cfg.DisableHTTPCompat {
		return
	}
	n, legacy := s.plainCompat()
	if n > 0 {
		s.log.Warn(fmt.Sprintf("%d serveur(s) appairé(s) en HTTP avant le passage à HTTPS : HTTP non chiffré reste accepté pour eux jusqu'à leur réappairage (Réglages › Agents › Réappairer). http_compat = false pour l'interdire.", n))
	}
	if legacy {
		s.log.Warn("Le jeton manuel agent.token antérieur à HTTPS reste accepté en HTTP (compatibilité) ; supprimez agent.token (nouveau jeton) ou réglez http_compat = false pour l'interdire.")
	}
}

// plainCompat counts the servers allowed to use plain HTTP and tells
// whether the legacy token is.
func (s *Server) plainCompat() (servers int, legacyToken bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sv := range s.state.Servers {
		if sv.Plain {
			servers++
		}
	}
	return servers, s.state.LegacyTokenPlain
}

// plainAllowed reports whether caller c may use plain HTTP while the agent
// serves HTTPS (compatibility with what was paired before HTTPS).
func (s *Server) plainAllowed(c caller) bool {
	if s.cfg.DisableHTTPCompat {
		return false
	}
	if c.legacy {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.state.LegacyTokenPlain
	}
	return c.plain
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
		c, ok := s.matchToken(sum)
		if tok == "" || !ok {
			if s.limiter.fail(ip) {
				s.log.Warn("authentification refusée, adresse temporairement bloquée", "ip", ip)
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="aether-agent"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "jeton manquant ou invalide")
			return
		}
		if s.tls != nil && r.TLS == nil {
			if !s.plainAllowed(c) {
				writeError(w, http.StatusUpgradeRequired, "tls_required", "cet agent exige HTTPS : utilisez https:// (réappairez l'agent depuis le serveur)")
				return
			}
			key := "legacy"
			if !c.legacy {
				key = "server:" + c.serverID
			}
			if _, seen := s.plainWarned.LoadOrStore(key, struct{}{}); !seen {
				s.log.Warn("appel en HTTP non chiffré accepté (appairage antérieur à HTTPS) : réappairez ce serveur pour passer en HTTPS", "appelant", key, "ip", ip)
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, c)))
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
	UID         string      `json:"uid"`
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Hostname    string      `json:"hostname"`
	Shares      []infoShare `json:"shares"`
	FFprobe     bool        `json:"ffprobe"`
	TLS         bool        `json:"tls"`
	Fingerprint string      `json:"fingerprint,omitempty"`
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	shares := s.activeShares()
	resp := infoResponse{
		UID:      s.uid,
		Name:     s.name,
		Version:  s.version,
		Hostname: s.hostname,
		Shares:   make([]infoShare, 0, len(shares)),
		FFprobe:  s.ffprobePath != "",
		TLS:      s.tls != nil,
	}
	resp.Fingerprint = s.Fingerprint()
	for _, sh := range shares {
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
