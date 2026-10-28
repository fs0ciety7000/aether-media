package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agentproto"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
	"github.com/fs0ciety7000/aether-media/internal/vfs/agentfs"
)

// Input is what clients send to create or update a source (see docs/API.md).
type Input struct {
	Kind  string     `json:"kind"`
	Name  string     `json:"name"`
	Root  string     `json:"root"`
	Agent *AgentIn   `json:"agent,omitempty"`
	SFTP  *SFTPInput `json:"sftp,omitempty"`
}

type AgentIn struct {
	AgentID int64  `json:"agentId"` // a paired agent (preferred); else URL + token
	URL     string `json:"url"`
	Token   string `json:"token"`
	Share   string `json:"share"`
	// Fingerprint (optional, manual token over https): SHA-256 of the
	// agent's certificate as shown by the agent. Absent: trusted on first
	// use (TOFU) unless a recognised CA signed it.
	Fingerprint string `json:"fingerprint,omitempty"`
}

type SFTPInput struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	Password   string `json:"password"`
	PrivateKey string `json:"privateKey"`
	Passphrase string `json:"passphrase"`
	HostKey    string `json:"hostKey"`
}

// ValidationError is a user-facing input problem (HTTP 400). Code, when
// set, is the API error code (default "invalid_source").
type ValidationError struct {
	Msg  string
	Code string
}

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

// prepared is a validated input ready to be stored.
type prepared struct {
	kind, name, root string
	cfg              Config
	sec              Secrets
}

// prepare validates an input. prev, when updating, provides the secrets to
// keep when the client leaves them empty.
func prepare(in Input, prev *Secrets) (prepared, error) {
	p := prepared{kind: in.Kind, name: strings.TrimSpace(in.Name)}
	if prev != nil {
		p.sec = *prev
	}
	switch in.Kind {
	case KindLocal:
		root := filepath.Clean(strings.TrimSpace(in.Root))
		if !filepath.IsAbs(root) {
			return p, invalid("chemin absolu requis : %s", in.Root)
		}
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			return p, invalid("dossier introuvable sur le serveur : %s", in.Root)
		}
		p.root = root
		if p.name == "" {
			p.name = root
		}
	case KindAgent:
		a := in.Agent
		if a == nil {
			return p, invalid("paramètres de l'agent manquants")
		}
		if a.AgentID != 0 {
			if strings.TrimSpace(a.Share) == "" {
				return p, invalid("partage de l'agent requis")
			}
			root, err := cleanRemote(in.Root, false)
			if err != nil {
				return p, err
			}
			p.root = root
			p.cfg.Agent = &AgentConfig{AgentID: a.AgentID, Share: strings.TrimSpace(a.Share)}
			p.sec = Secrets{} // credentials live with the paired agent
			break
		}
		u, err := url.Parse(strings.TrimSpace(a.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return p, invalid("URL de l'agent invalide (ex. https://nas.local:8097)")
		}
		if strings.TrimSpace(a.Share) == "" {
			return p, invalid("partage de l'agent requis")
		}
		fp, err := agentproto.NormalizeFingerprint(a.Fingerprint)
		if err != nil {
			return p, invalid("%s", err.Error())
		}
		if fp != "" && u.Scheme != "https" {
			return p, invalid("une empreinte de certificat suppose une adresse https://")
		}
		if a.Token != "" {
			p.sec.Token = strings.TrimSpace(a.Token)
		}
		if p.sec.Token == "" {
			return p, invalid("jeton d'appairage de l'agent requis")
		}
		root, err := cleanRemote(in.Root, false)
		if err != nil {
			return p, err
		}
		p.root = root
		p.cfg.Agent = &AgentConfig{URL: strings.TrimSuffix(u.String(), "/"), Share: strings.TrimSpace(a.Share), Fingerprint: fp}
		if p.name == "" {
			p.name = u.Host + " · " + path.Join(a.Share, root)
		}
	case KindSFTP:
		sf := in.SFTP
		if sf == nil {
			return p, invalid("paramètres SFTP manquants")
		}
		if strings.TrimSpace(sf.Host) == "" || strings.TrimSpace(sf.User) == "" {
			return p, invalid("hôte et utilisateur SFTP requis")
		}
		port := sf.Port
		if port == 0 {
			port = 22
		}
		if port < 1 || port > 65535 {
			return p, invalid("port SFTP invalide")
		}
		if sf.Password != "" {
			p.sec.Password, p.sec.PrivateKey, p.sec.Passphrase = sf.Password, "", ""
		}
		if sf.PrivateKey != "" {
			p.sec.PrivateKey, p.sec.Passphrase, p.sec.Password = sf.PrivateKey, sf.Passphrase, ""
		}
		auth := "password"
		if p.sec.PrivateKey != "" {
			auth = "key"
		} else if p.sec.Password == "" {
			return p, invalid("mot de passe ou clé privée SFTP requis")
		}
		root, err := cleanRemote(in.Root, true)
		if err != nil {
			return p, err
		}
		p.root = root
		p.cfg.SFTP = &SFTPConfig{Host: strings.TrimSpace(sf.Host), Port: port, User: strings.TrimSpace(sf.User), Auth: auth, HostKey: strings.TrimSpace(sf.HostKey)}
		if p.name == "" {
			p.name = fmt.Sprintf("%s@%s:%s", p.cfg.SFTP.User, p.cfg.SFTP.Host, root)
		}
	default:
		return p, invalid("type de source attendu : local, agent ou sftp")
	}
	if len(p.name) > 200 {
		p.name = p.name[:200]
	}
	return p, nil
}

// cleanRemote normalises a remote root ("" or "/" allowed for agents).
func cleanRemote(root string, absolute bool) (string, error) {
	root = strings.TrimSpace(root)
	if strings.ContainsRune(root, 0) {
		return "", invalid("chemin invalide")
	}
	if absolute {
		if !strings.HasPrefix(root, "/") {
			return "", invalid("chemin distant absolu requis (ex. /home/user/media)")
		}
		return path.Clean(root), nil
	}
	c := strings.TrimPrefix(path.Clean("/"+root), "/")
	for _, seg := range strings.Split(root, "/") {
		if seg == ".." {
			return "", invalid("chemin invalide : %s", root)
		}
	}
	return c, nil
}

// TestResult is returned by the connection test.
type TestResult struct {
	OK      bool       `json:"ok"`
	Error   string     `json:"error,omitempty"`
	HostKey string     `json:"hostKey,omitempty"`
	Agent   *AgentInfo `json:"agent,omitempty"`
	// Fingerprint is the agent certificate's SHA-256 (manual token over
	// https); Warning "fingerprint_tofu" means it was not given and will be
	// pinned as first seen.
	Fingerprint string `json:"fingerprint,omitempty"`
	Warning     string `json:"warning,omitempty"`
}

// WarnTOFU: the certificate of a manual-token agent is trusted on first use.
const WarnTOFU = "fingerprint_tofu"

// tofu pins, for a manual-token https agent without fingerprint, the
// certificate seen now, unless the system CAs vouch for it.
func (m *Manager) tofu(ctx context.Context, ac *AgentConfig, res *TestResult) error {
	if ac.AgentID != 0 || !strings.HasPrefix(ac.URL, "https://") {
		return nil
	}
	if ac.Fingerprint != "" {
		res.Fingerprint = ac.Fingerprint
		return nil
	}
	ti, err := agentfs.ProbeTLS(ctx, ac.URL)
	if err != nil {
		return err
	}
	if ti.CAValid {
		return nil
	}
	ac.Fingerprint = ti.Fingerprint
	res.Fingerprint, res.Warning = ti.Fingerprint, WarnTOFU
	if m.log != nil {
		m.log.Warn("agent (jeton manuel) : certificat accepté à la première connexion (TOFU), vérifiez son empreinte", "url", ac.URL, "empreinte", agentproto.FormatFingerprint(ti.Fingerprint))
	}
	return nil
}

// Test checks that a source can be reached and listed.
func (m *Manager) Test(ctx context.Context, in Input) (TestResult, error) {
	// An agent without a share yet: just connect and list its shares.
	if in.Kind == KindAgent && in.Agent != nil && strings.TrimSpace(in.Agent.Share) == "" {
		probeIn := in
		agentIn := *in.Agent
		agentIn.Share = "-"
		probeIn.Agent = &agentIn
		p, err := prepare(probeIn, nil)
		if err != nil {
			return TestResult{}, err
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		var res TestResult
		if err := m.tofu(ctx, p.cfg.Agent, &res); err != nil {
			return TestResult{Error: err.Error()}, nil
		}
		info, err := m.factory.AgentInfo(ctx, p.cfg.Agent.URL, p.sec.Token, p.cfg.Agent.Fingerprint)
		if err != nil {
			res.Error = err.Error()
			return res, nil
		}
		res.OK, res.Agent = true, &info
		return res, nil
	}
	p, err := prepare(in, nil)
	if err != nil {
		return TestResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	res := TestResult{}
	switch p.kind {
	case KindAgent:
		ac, asec, err := m.agentCreds(ctx, *p.cfg.Agent, p.sec)
		if err != nil {
			return TestResult{Error: err.Error()}, nil
		}
		if err := m.tofu(ctx, &ac, &res); err != nil {
			return TestResult{Error: err.Error()}, nil
		}
		p.cfg.Agent.Fingerprint = ac.Fingerprint
		if p.cfg.Agent.AgentID != 0 {
			p.cfg.Agent.Fingerprint = "" // lives with the paired agent
		}
		info, err := m.factory.AgentInfo(ctx, ac.URL, asec.Token, ac.Fingerprint)
		if err != nil {
			res.Error = err.Error()
			return res, nil
		}
		res.Agent = &info
		found := false
		for _, s := range info.Shares {
			found = found || s == p.cfg.Agent.Share
		}
		if !found {
			return TestResult{Error: fmt.Sprintf("partage %q inconnu de l'agent", p.cfg.Agent.Share), Agent: &info}, nil
		}
	case KindSFTP:
		fp, err := m.factory.SFTPHostKey(ctx, *p.cfg.SFTP)
		if err != nil {
			return TestResult{Error: err.Error()}, nil
		}
		res.HostKey = fp
		if p.cfg.SFTP.HostKey == "" {
			return TestResult{Error: "host_key_unknown", HostKey: fp}, nil
		}
		if p.cfg.SFTP.HostKey != fp {
			return TestResult{Error: "host_key_mismatch", HostKey: fp}, nil
		}
	}
	fsys, err := m.openPrepared(ctx, p)
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	defer func() { _ = fsys.Close() }()
	if err := fsys.Ping(ctx); err != nil {
		res.Error = err.Error()
		return res, nil
	}
	res.OK = true
	return res, nil
}

func (m *Manager) openPrepared(ctx context.Context, p prepared) (vfs.FS, error) {
	switch p.kind {
	case KindLocal:
		return m.factory.Local(p.root)
	case KindAgent:
		ac, asec, err := m.agentCreds(ctx, *p.cfg.Agent, p.sec)
		if err != nil {
			return nil, err
		}
		return m.factory.Agent(ctx, ac, asec, p.root)
	case KindSFTP:
		return m.factory.SFTP(ctx, *p.cfg.SFTP, p.sec, p.root)
	}
	return nil, errors.New("unknown kind")
}

// Create validates and stores a new source of a library. Remote sources
// must pass a connection test first.
func (m *Manager) Create(ctx context.Context, q *db.Queries, libID int64, in Input) (db.LibrarySource, error) {
	p, err := prepare(in, nil)
	if err != nil {
		return db.LibrarySource{}, err
	}
	if p.kind != KindLocal {
		res, err := m.Test(ctx, in)
		if err != nil {
			return db.LibrarySource{}, err
		}
		if !res.OK {
			return db.LibrarySource{}, invalid("connexion impossible : %s", res.Error)
		}
		if res.Warning == WarnTOFU && p.cfg.Agent != nil && p.cfg.Agent.AgentID == 0 {
			p.cfg.Agent.Fingerprint = res.Fingerprint
		}
		if res.Agent != nil {
			p.cfg.Agent.Hostname, p.cfg.Agent.Version = res.Agent.Hostname, res.Agent.Version
			if strings.TrimSpace(in.Name) == "" && p.cfg.Agent.AgentID != 0 {
				p.name = res.Agent.Hostname + " · " + path.Join(p.cfg.Agent.Share, p.root)
			}
		}
	}
	return m.insert(ctx, q, libID, p)
}

func (m *Manager) insert(ctx context.Context, q *db.Queries, libID int64, p prepared) (db.LibrarySource, error) {
	cfg, _ := json.Marshal(p.cfg)
	var sealed []byte
	if p.sec != (Secrets{}) {
		var err error
		if sealed, err = m.box.SealJSON(p.sec); err != nil {
			return db.LibrarySource{}, err
		}
	}
	return q.CreateSource(ctx, db.CreateSourceParams{LibraryID: libID, Kind: p.kind, Name: p.name, Root: p.root, Config: string(cfg), Secret: sealed})
}

// Update changes a source; empty secrets keep the stored ones.
func (m *Manager) Update(ctx context.Context, src db.LibrarySource, in Input) (db.LibrarySource, error) {
	_, prevSec, err := m.Decode(src)
	if err != nil {
		return src, err
	}
	in.Kind = src.Kind
	p, err := prepare(in, &prevSec)
	if err != nil {
		return src, err
	}
	prevCfg, _, _ := m.Decode(src)
	sameTarget := p.root == src.Root && p.sec == prevSec && sameConfig(p.cfg, prevCfg)
	if p.kind != KindLocal && !sameTarget {
		test := in
		if test.Agent != nil && test.Agent.Token == "" {
			test.Agent.Token = prevSec.Token
		}
		if test.Agent != nil && test.Agent.Fingerprint == "" && p.cfg.Agent != nil && prevCfg.Agent != nil &&
			p.cfg.Agent.URL == prevCfg.Agent.URL {
			a := *test.Agent
			a.Fingerprint = prevCfg.Agent.Fingerprint // same address: keep the pin, no new TOFU
			test.Agent = &a
		}
		if test.SFTP != nil && test.SFTP.Password == "" && test.SFTP.PrivateKey == "" {
			test.SFTP.Password, test.SFTP.PrivateKey, test.SFTP.Passphrase = prevSec.Password, prevSec.PrivateKey, prevSec.Passphrase
		}
		res, err := m.Test(ctx, test)
		if err != nil {
			return src, err
		}
		if !res.OK {
			return src, invalid("connexion impossible : %s", res.Error)
		}
		if res.Warning == WarnTOFU && p.cfg.Agent != nil && p.cfg.Agent.AgentID == 0 {
			p.cfg.Agent.Fingerprint = res.Fingerprint
		}
	}
	if p.cfg.Agent != nil && prevCfg.Agent != nil && p.cfg.Agent.AgentID == 0 && p.cfg.Agent.Fingerprint == "" &&
		p.cfg.Agent.URL == prevCfg.Agent.URL {
		p.cfg.Agent.Fingerprint = prevCfg.Agent.Fingerprint // unchanged address keeps its pin
	}
	if p.cfg.Agent != nil && prevCfg.Agent != nil && sameTarget {
		p.cfg.Agent.Hostname, p.cfg.Agent.Version = prevCfg.Agent.Hostname, prevCfg.Agent.Version
	}
	cfg, _ := json.Marshal(p.cfg)
	sealed, err := m.box.SealJSON(p.sec)
	if err != nil {
		return src, err
	}
	out, err := m.store.UpdateSource(ctx, db.UpdateSourceParams{Name: p.name, Root: p.root, Config: string(cfg), Secret: sealed, ID: src.ID, LibraryID: src.LibraryID})
	if err == nil {
		m.Forget(src.ID)
	}
	return out, err
}

// sameConfig compares the connection settings (ignoring informative fields).
func sameConfig(a, b Config) bool {
	switch {
	case a.Agent != nil && b.Agent != nil:
		return a.Agent.URL == b.Agent.URL && a.Agent.Share == b.Agent.Share &&
			(a.Agent.Fingerprint == "" || a.Agent.Fingerprint == b.Agent.Fingerprint)
	case a.SFTP != nil && b.SFTP != nil:
		return *a.SFTP == *b.SFTP
	}
	return a.Agent == nil && b.Agent == nil && a.SFTP == nil && b.SFTP == nil
}

// TestStored checks a saved source with its stored credentials.
func (m *Manager) TestStored(ctx context.Context, src db.LibrarySource) TestResult {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	m.Forget(src.ID)
	f, _, err := m.FS(ctx, src.ID)
	if err == nil {
		err = f.Ping(ctx)
	}
	m.SetStatus(ctx, src, err)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	return TestResult{OK: true}
}
