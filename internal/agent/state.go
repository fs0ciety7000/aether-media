package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	stateFileName = "agent-state.json"
	maxNameLen    = 100
	// stateVersion 2: HTTPS by default. States written before it (no
	// "version") come from agents that only spoke plain HTTP.
	stateVersion = 2
)

// pairedServer is an Aether server paired with a code. Only the SHA-256 of
// its token is kept.
type pairedServer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"tokenHash"` // hex SHA-256
	PairedAt  time.Time `json:"pairedAt"`
	// Plain: paired over plain HTTP (before the agent switched to HTTPS, or
	// with tls = false). Such a server may keep calling over HTTP until it
	// pairs again; pairing over HTTPS clears it.
	Plain bool `json:"plain,omitempty"`
}

// agentState is <data>/agent-state.json.
type agentState struct {
	Version int            `json:"version"`
	UID     string         `json:"uid"`
	Name    string         `json:"name"`
	Shares  []Share        `json:"shares"` // managed shares (UI), possibly unavailable
	Servers []pairedServer `json:"servers"`
	// LegacyTokenPlain: agent.token predates HTTPS, so a server may use it
	// over plain HTTP (manual pairing). Cleared when agent.token is
	// regenerated.
	LegacyTokenPlain bool `json:"legacyTokenPlain,omitempty"`
}

// newUID returns a random RFC 4122 version 4 UUID.
func newUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

// loadState reads the state file. A missing file yields a fresh state
// (created=true); a corrupt one is an error rather than being silently
// replaced (it holds the pairings).
func loadState(dir string) (st agentState, created bool, err error) {
	p := filepath.Join(dir, stateFileName)
	b, err := os.ReadFile(p)
	switch {
	case errors.Is(err, os.ErrNotExist):
		uid, err := newUID()
		if err != nil {
			return agentState{}, false, err
		}
		return agentState{UID: uid}, true, nil // Version 0: upgraded by the caller
	case err != nil:
		return agentState{}, false, fmt.Errorf("agent: lecture de %s : %w", p, err)
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return agentState{}, false, fmt.Errorf("agent: %s illisible (%w) ; corrigez-le ou supprimez-le (les appairages seront perdus)", p, err)
	}
	if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(p, 0o600); err != nil {
			return agentState{}, false, fmt.Errorf("agent: permissions de %s : %w", p, err)
		}
	}
	if st.UID == "" {
		if st.UID, err = newUID(); err != nil {
			return agentState{}, false, err
		}
		created = true
	}
	// Drop malformed entries instead of refusing to start.
	servers := st.Servers[:0]
	for _, sv := range st.Servers {
		if h, err := hex.DecodeString(sv.TokenHash); err == nil && len(h) == 32 && sv.ID != "" {
			servers = append(servers, sv)
		}
	}
	st.Servers = servers
	return st, created, nil
}

// saveState writes the state atomically (temporary file + rename, 0600).
func saveState(dir string, st agentState) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("agent: création de %s : %w", dir, err)
	}
	if st.Shares == nil {
		st.Shares = []Share{}
	}
	if st.Servers == nil {
		st.Servers = []pairedServer{}
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".agent-state-*.tmp") // created 0600
	if err != nil {
		return fmt.Errorf("agent: écriture de l'état : %w", err)
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("agent: écriture de l'état : %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return fmt.Errorf("agent: écriture de l'état : %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("agent: écriture de l'état : %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("agent: écriture de l'état : %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, stateFileName)); err != nil {
		return fmt.Errorf("agent: écriture de l'état : %w", err)
	}
	ok = true
	if d, err := os.Open(dir); err == nil { // persist the rename
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// saveLocked persists s.state. Callers hold s.mu.
func (s *Server) saveLocked() error {
	return saveState(s.cfg.DataDir, s.state)
}
