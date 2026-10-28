// Package sources manages where a library's files live (local folders,
// remote Aether agents, SFTP servers) and resolves the "locations" stored in
// the database to readable files and FFmpeg inputs.
//
// Locations: files of local sources keep their absolute OS path; files of
// remote sources are stored as "src:<sourceID>:/<relative path>", which
// behaves like a path for filepath.Dir/Base/Join.
package sources

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/metadata/nfo"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// Kinds of sources.
const (
	KindLocal = "local"
	KindAgent = "agent"
	KindSFTP  = "sftp"
)

const remotePrefix = "src:"

// AgentConfig and SFTPConfig are the non-secret settings stored in clear.
type AgentConfig struct {
	AgentID  int64  `json:"agentId,omitempty"` // paired agent (remote_agents); URL/token come from it
	URL      string `json:"url"`
	Share    string `json:"share"`
	Hostname string `json:"hostname,omitempty"`
	Version  string `json:"version,omitempty"`
	// Fingerprint pins the certificate of a manual-token agent (lowercase
	// hex SHA-256; paired agents keep theirs in remote_agents).
	Fingerprint string `json:"fingerprint,omitempty"`
}

type SFTPConfig struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Auth    string `json:"auth"` // password | key
	HostKey string `json:"hostKey"`
}

// Secrets are stored encrypted.
type Secrets struct {
	Token      string `json:"token,omitempty"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

// Config is the decoded form of a source row.
type Config struct {
	Agent *AgentConfig `json:"agent,omitempty"`
	SFTP  *SFTPConfig  `json:"sftp,omitempty"`
}

// Factory builds file systems for remote kinds (wired in main so this
// package does not depend on every backend).
type Factory interface {
	Agent(ctx context.Context, cfg AgentConfig, sec Secrets, root string) (vfs.FS, error)
	SFTP(ctx context.Context, cfg SFTPConfig, sec Secrets, root string) (vfs.FS, error)
	Local(root string) (vfs.FS, error)
	// AgentEvents streams change notifications of an agent share until ctx
	// is done (reconnecting as needed).
	AgentEvents(ctx context.Context, cfg AgentConfig, sec Secrets) (<-chan []string, error)
	// AgentInfo queries an agent (connection test, share list), pinning
	// its certificate when fingerprint is set.
	AgentInfo(ctx context.Context, url, token, fingerprint string) (AgentInfo, error)
	// SFTPHostKey connects only to read the server's host key fingerprint.
	SFTPHostKey(ctx context.Context, cfg SFTPConfig) (string, error)
}

// AgentInfo describes a reachable agent.
type AgentInfo struct {
	Hostname string   `json:"hostname"`
	Version  string   `json:"version"`
	Shares   []string `json:"shares"`
	TLS      bool     `json:"tls"`
}

// Manager caches open file systems and tracks source health.
type Manager struct {
	store   *store.Store
	box     *secret.Box
	factory Factory
	hub     *events.Hub
	log     *slog.Logger

	// ServerID and ServerName identify this server to agents when pairing.
	ServerID, ServerName string

	// ProxyBase is the server's own base URL (e.g. http://127.0.0.1:8096),
	// used to expose files of sources FFmpeg cannot read directly.
	ProxyBase string
	proxyKey  []byte

	// AgentConcurrency bounds the concurrent analyses and scan walks per
	// agent (DefaultAgentConcurrency when 0). Set it before first use.
	AgentConcurrency int
	// OnAgentOnline is called when an agent found unreachable answers again
	// (e.g. to wake the postponed probe jobs).
	OnAgentOnline func()

	mu  sync.Mutex
	fss map[int64]vfs.FS

	gmu          sync.Mutex
	gates        map[string]*agentGate // agentKey → gate
	gatesStopped bool
}

func New(s *store.Store, box *secret.Box, f Factory, hub *events.Hub, log *slog.Logger) *Manager {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return &Manager{store: s, box: box, factory: f, hub: hub, log: log, proxyKey: key, fss: map[int64]vfs.FS{}, gates: map[string]*agentGate{}}
}

// Location builds the stored location of a file of src.
func Location(src db.LibrarySource, rel string) string {
	if src.Kind == KindLocal {
		return filepath.Join(src.Root, filepath.FromSlash(rel))
	}
	return RootLocation(src) + "/" + rel
}

// RootLocation is the location of a source's root directory.
func RootLocation(src db.LibrarySource) string {
	if src.Kind == KindLocal {
		return src.Root
	}
	return remotePrefix + strconv.FormatInt(src.ID, 10) + ":"
}

// ParseRemote splits a remote location into source id and relative path.
// ok is false for local paths.
func ParseRemote(loc string) (id int64, rel string, ok bool) {
	if !strings.HasPrefix(loc, remotePrefix) {
		return 0, "", false
	}
	rest := strings.ReplaceAll(loc[len(remotePrefix):], `\`, "/")
	idStr, rel, found := strings.Cut(rest, ":")
	if !found {
		return 0, "", false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return id, strings.TrimPrefix(path.Clean("/"+rel), "/"), true
}

// IsRemote reports whether a location belongs to a remote source.
func IsRemote(loc string) bool { _, _, ok := ParseRemote(loc); return ok }

// Decode returns the config and secrets of a source row.
func (m *Manager) Decode(src db.LibrarySource) (Config, Secrets, error) {
	var cfg Config
	var sec Secrets
	if err := json.Unmarshal([]byte(src.Config), &cfg); err != nil {
		return cfg, sec, fmt.Errorf("source %d config: %w", src.ID, err)
	}
	err := m.box.OpenJSON(src.Secret, &sec)
	return cfg, sec, err
}

// FS returns the (cached) file system of a source.
func (m *Manager) FS(ctx context.Context, sourceID int64) (vfs.FS, db.LibrarySource, error) {
	src, err := m.store.GetSource(ctx, sourceID)
	if err != nil {
		return nil, src, err
	}
	m.mu.Lock()
	f, ok := m.fss[sourceID]
	m.mu.Unlock()
	if ok {
		return f, src, nil
	}
	f, err = m.open(ctx, src)
	if err != nil {
		return nil, src, err
	}
	m.mu.Lock()
	if existing, ok := m.fss[sourceID]; ok {
		m.mu.Unlock()
		_ = f.Close()
		return existing, src, nil
	}
	m.fss[sourceID] = f
	m.mu.Unlock()
	return f, src, nil
}

func (m *Manager) open(ctx context.Context, src db.LibrarySource) (vfs.FS, error) {
	cfg, sec, err := m.Decode(src)
	if err != nil {
		return nil, err
	}
	switch src.Kind {
	case KindLocal:
		return m.factory.Local(src.Root)
	case KindAgent:
		if cfg.Agent == nil {
			return nil, errors.New("agent source without config")
		}
		ac, asec, err := m.agentCreds(ctx, *cfg.Agent, sec)
		if err != nil {
			return nil, err
		}
		f, err := m.factory.Agent(ctx, ac, asec, src.Root)
		if err != nil {
			return nil, err
		}
		return gatedFS{FS: f, g: m.gate(ac, asec)}, nil
	case KindSFTP:
		if cfg.SFTP == nil {
			return nil, errors.New("sftp source without config")
		}
		return m.factory.SFTP(ctx, *cfg.SFTP, sec, src.Root)
	}
	return nil, fmt.Errorf("unknown source kind %q", src.Kind)
}

// Forget closes and drops a cached file system (after an update/delete).
func (m *Manager) Forget(sourceID int64) {
	m.mu.Lock()
	f, ok := m.fss[sourceID]
	delete(m.fss, sourceID)
	m.mu.Unlock()
	if ok {
		_ = f.Close()
	}
}

// Close releases every open file system.
func (m *Manager) Close() {
	m.stopGates()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, f := range m.fss {
		_ = f.Close()
		delete(m.fss, id)
	}
}

// resolve maps a location to a file system and a path inside it. Local
// locations are served by the local source that contains them when
// sourceID is unknown (0).
func (m *Manager) resolve(ctx context.Context, loc string, sourceID int64) (vfs.FS, string, error) {
	if id, rel, ok := ParseRemote(loc); ok {
		f, _, err := m.FS(ctx, id)
		return f, rel, err
	}
	if sourceID != 0 {
		f, src, err := m.FS(ctx, sourceID)
		if err != nil {
			return nil, "", err
		}
		rel, err := filepath.Rel(src.Root, loc)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, "", vfs.ErrPath
		}
		return f, filepath.ToSlash(rel), nil
	}
	return nil, "", errLocal
}

var errLocal = errors.New("local location")

// Open opens a file by location (any kind of source).
func (m *Manager) Open(ctx context.Context, loc string, sourceID int64) (vfs.File, error) {
	f, rel, err := m.resolve(ctx, loc, sourceID)
	if errors.Is(err, errLocal) {
		return openLocal(loc)
	}
	if err != nil {
		return nil, err
	}
	return f.Open(ctx, rel)
}

// ReadDir lists a directory by location.
func (m *Manager) ReadDir(ctx context.Context, loc string) ([]vfs.Entry, error) {
	f, rel, err := m.resolve(ctx, loc, 0)
	if errors.Is(err, errLocal) {
		return readDirLocal(loc)
	}
	if err != nil {
		return nil, err
	}
	return f.ReadDir(ctx, rel)
}

// Stat describes a file by location.
func (m *Manager) Stat(ctx context.Context, loc string) (vfs.Entry, error) {
	f, rel, err := m.resolve(ctx, loc, 0)
	if errors.Is(err, errLocal) {
		return statLocal(loc)
	}
	if err != nil {
		return vfs.Entry{}, err
	}
	return f.Stat(ctx, rel)
}

// InputURL returns what FFmpeg/ffprobe should read for a location: the local
// path, or a short-lived signed loopback URL of the server's internal proxy
// (agents, SFTP). FFmpeg never connects to an agent itself: it could not
// check the pinned certificate and the token would show on its command line.
func (m *Manager) InputURL(ctx context.Context, loc string) (string, error) {
	id, rel, ok := ParseRemote(loc)
	if !ok {
		return loc, nil
	}
	f, _, err := m.FS(ctx, id)
	if err != nil {
		return "", err
	}
	if u := f.InputURL(rel); u != "" {
		return u, nil
	}
	return m.proxyURL(id, rel, 12*time.Hour), nil
}

// proxyURL signs (source, path, expiry) so only the server's own FFmpeg can
// use it.
func (m *Manager) proxyURL(id int64, rel string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%d|%s|%d", id, rel, exp)
	mac := hmac.New(sha256.New, m.proxyKey)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s/internal/vfs/%d/%d/%s/%s", m.ProxyBase, id, exp, sig, escapePath(rel))
}

// VerifyProxy checks a proxy request and returns the location to serve.
func (m *Manager) VerifyProxy(id int64, exp int64, sig, rel string) (string, bool) {
	if time.Now().Unix() > exp {
		return "", false
	}
	mac := hmac.New(sha256.New, m.proxyKey)
	fmt.Fprintf(mac, "%d|%s|%d", id, rel, exp)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return "", false
	}
	return remotePrefix + strconv.FormatInt(id, 10) + ":/" + rel, true
}

func escapePath(rel string) string {
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = urlPathEscape(p)
	}
	return strings.Join(parts, "/")
}

// SetStatus records a source's health and notifies clients when it changes.
func (m *Manager) SetStatus(ctx context.Context, src db.LibrarySource, err error) {
	status, msg := "online", ""
	if err != nil {
		status, msg = "offline", err.Error()
	}
	if serr := m.store.SetSourceStatus(ctx, db.SetSourceStatusParams{Status: status, LastError: msg, Column3: status, ID: src.ID}); serr != nil {
		m.log.Error("statut de source", "source", src.ID, "err", serr)
		return
	}
	if src.Status != status {
		m.log.Info("source "+map[string]string{"online": "en ligne", "offline": "hors ligne"}[status], "source", src.ID, "name", src.Name, "err", msg)
		m.hub.PublishAdmin("source.status", map[string]any{"id": src.ID, "libraryId": src.LibraryID, "status": status, "lastError": msg})
	}
}

// Monitor pings every source periodically until ctx is done.
func (m *Manager) Monitor(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		m.PingAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// PingAll checks every agent and source once.
func (m *Manager) PingAll(ctx context.Context) {
	m.pingAgents(ctx)
	srcs, err := m.store.ListAllSources(ctx)
	if err != nil {
		return
	}
	for _, src := range srcs {
		pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		f, _, err := m.FS(pctx, src.ID)
		if err == nil {
			err = f.Ping(pctx)
		}
		if err != nil {
			m.Forget(src.ID) // reconnect from scratch next time
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		m.SetStatus(ctx, src, err)
	}
}

// NFOFileSystem adapts the manager for the NFO provider: local paths go to
// the disk, remote locations to their source.
func (m *Manager) NFOFileSystem() nfo.FileSystem { return nfoFS{m: m, os: nfo.OSFileSystem()} }

type nfoFS struct {
	m  *Manager
	os nfo.FileSystem
}

const remoteOpTimeout = 30 * time.Second

func (f nfoFS) ReadDir(dir string) ([]nfo.DirEntry, error) {
	if !IsRemote(dir) {
		return f.os.ReadDir(dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteOpTimeout)
	defer cancel()
	entries, err := f.m.ReadDir(ctx, dir)
	if err != nil {
		return nil, err
	}
	out := make([]nfo.DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, nfo.DirEntry{Name: path.Base(e.Path), IsDir: e.IsDir})
	}
	return out, nil
}

func (f nfoFS) IsDir(p string) (bool, error) {
	if !IsRemote(p) {
		return f.os.IsDir(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteOpTimeout)
	defer cancel()
	e, err := f.m.Stat(ctx, p)
	return e.IsDir, err
}

func (f nfoFS) Open(p string) (io.ReadCloser, error) {
	if !IsRemote(p) {
		return f.os.Open(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteOpTimeout)
	file, err := f.m.Open(ctx, p, 0)
	if err != nil {
		cancel()
		return nil, err
	}
	return cancelOnClose{File: file, cancel: cancel}, nil
}

type cancelOnClose struct {
	vfs.File
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error { defer c.cancel(); return c.File.Close() }

// Changes notifies (with the changed paths) when an agent source reports
// file changes. Only agent sources support it.
func (m *Manager) Changes(ctx context.Context, sourceID int64) (<-chan []string, error) {
	src, err := m.store.GetSource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if src.Kind != KindAgent {
		return nil, errors.New("change events are only available for agent sources")
	}
	cfg, sec, err := m.Decode(src)
	if err != nil {
		return nil, err
	}
	if cfg.Agent == nil {
		return nil, errors.New("agent source without config")
	}
	ac, asec, err := m.agentCreds(ctx, *cfg.Agent, sec)
	if err != nil {
		return nil, err
	}
	return m.factory.AgentEvents(ctx, ac, asec)
}

// Probe analyses a media file wherever it lives. Agents run ffprobe next to
// the file (only the JSON travels); other sources are probed through their
// FFmpeg input URL.
func (m *Manager) Probe(ctx context.Context, p *probe.Prober, loc string) (*probe.Info, error) {
	id, rel, ok := ParseRemote(loc)
	if !ok {
		return p.Probe(ctx, loc)
	}
	src, err := m.store.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	if src.Kind == KindAgent {
		cfg, sec, err := m.Decode(src)
		if err != nil {
			return nil, err
		}
		if cfg.Agent == nil {
			return nil, errors.New("agent source without config")
		}
		ac, asec, err := m.agentCreds(ctx, *cfg.Agent, sec)
		if err != nil {
			return nil, err
		}
		// At most AgentConcurrency analyses per agent, none while it is
		// unreachable (AgentDownError: the caller postpones).
		g := m.gate(ac, asec)
		release, err := g.acquire(ctx)
		if err != nil {
			return nil, err
		}
		data, err := agentProbe(ctx, ac, asec, path.Join(src.Root, rel))
		release()
		if err = g.done(ctx, err); err != nil {
			return nil, err
		}
		return probe.ParseOutput(data, loc)
	}
	u, err := m.InputURL(ctx, loc)
	if err != nil {
		return nil, err
	}
	return p.Probe(ctx, u)
}

// NewForTests returns a manager backed by the real local backend and a
// throw-away secret key, for package tests elsewhere.
func NewForTests(s *store.Store, dir string, hub *events.Hub, log *slog.Logger) (*Manager, error) {
	box, err := secret.Open(dir)
	if err != nil {
		return nil, err
	}
	return New(s, box, DefaultFactory{}, hub, log), nil
}
