package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// agentCreds resolves the URL and token of an agent source: from the paired
// agent when AgentID is set, else from the source itself (legacy token).
func (m *Manager) agentCreds(ctx context.Context, cfg AgentConfig, sec Secrets) (AgentConfig, Secrets, error) {
	if cfg.AgentID == 0 {
		return cfg, sec, nil
	}
	a, err := m.store.GetAgent(ctx, cfg.AgentID)
	if err != nil {
		return cfg, sec, fmt.Errorf("agent %d: %w", cfg.AgentID, err)
	}
	var asec Secrets
	if err := m.box.OpenJSON(a.Secret, &asec); err != nil {
		return cfg, sec, err
	}
	cfg.URL = a.Url
	return cfg, asec, nil
}

// PairResult is what an agent returns when a pairing code is accepted.
type PairResult struct {
	Token   string `json:"token"`
	UID     string `json:"uid"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgentError carries the agent's error code ("invalid_code", "rate_limited"...).
type AgentError struct {
	Status int
	Code   string
	Msg    string
}

func (e *AgentError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("agent: HTTP %d %s", e.Status, e.Code)
}

// agentCall performs an agent API request and decodes the JSON response.
func agentCall(ctx context.Context, method, base, path, token string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(base, "/")+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := agentHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("agent injoignable : %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		var e struct{ Error, Message string }
		_ = json.Unmarshal(data, &e)
		return &AgentError{Status: res.StatusCode, Code: e.Error, Msg: e.Message}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Pair exchanges a pairing code displayed by an agent for a token and saves
// the agent (re-pairing an agent updates it).
func (m *Manager) Pair(ctx context.Context, rawURL, code string) (db.RemoteAgent, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return db.RemoteAgent{}, invalid("adresse de l'agent invalide (ex. http://192.168.1.20:8097)")
	}
	base := strings.TrimSuffix(u.String(), "/")
	code = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	if len(code) < 6 || len(code) > 16 {
		return db.RemoteAgent{}, invalid("code d'appairage invalide")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var res PairResult
	err = agentCall(ctx, http.MethodPost, base, "/agent/v1/pair", "", map[string]string{
		"code": code, "serverName": m.ServerName, "serverId": m.ServerID,
	}, &res)
	var ae *AgentError
	if errors.As(err, &ae) {
		switch ae.Status {
		case http.StatusUnauthorized, http.StatusBadRequest, http.StatusForbidden:
			return db.RemoteAgent{}, invalid("code d'appairage incorrect ou expiré")
		case http.StatusTooManyRequests:
			return db.RemoteAgent{}, invalid("trop d'essais : patientez quelques minutes")
		}
	}
	if ae == nil && err != nil {
		return db.RemoteAgent{}, fmt.Errorf("%w: agent injoignable: %v", vfs.ErrOffline, err) //nolint:errorlint // keep a single sentinel
	}
	if err != nil {
		return db.RemoteAgent{}, err
	}
	if res.Token == "" || res.UID == "" {
		return db.RemoteAgent{}, errors.New("réponse d'appairage incomplète")
	}
	sealed, err := m.box.SealJSON(Secrets{Token: res.Token})
	if err != nil {
		return db.RemoteAgent{}, err
	}
	a, err := m.store.UpsertAgent(ctx, db.UpsertAgentParams{Uid: res.UID, Name: res.Name, Url: base, Secret: sealed, Version: res.Version})
	if err != nil {
		return a, err
	}
	m.forgetAgentSources(ctx, a.ID) // new token: reopen file systems
	return a, nil
}

// AgentCall runs an authenticated request against a paired agent.
func (m *Manager) AgentCall(ctx context.Context, agentID int64, method, path string, body, out any) error {
	a, err := m.store.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	var sec Secrets
	if err := m.box.OpenJSON(a.Secret, &sec); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return agentCall(ctx, method, a.Url, path, sec.Token, body, out)
}

// Unpair revokes this server's token on the agent (best effort) and forgets it.
func (m *Manager) Unpair(ctx context.Context, agentID int64) error {
	_ = m.AgentCall(ctx, agentID, http.MethodDelete, "/agent/v1/pair", nil, nil)
	return m.store.DeleteAgent(ctx, agentID)
}

func (m *Manager) forgetAgentSources(ctx context.Context, agentID int64) {
	srcs, err := m.store.ListAllSources(ctx)
	if err != nil {
		return
	}
	for _, s := range srcs {
		var cfg Config
		if s.Kind == KindAgent && json.Unmarshal([]byte(s.Config), &cfg) == nil && cfg.Agent != nil && cfg.Agent.AgentID == agentID {
			m.Forget(s.ID)
		}
	}
}

// pingAgents refreshes the status of every paired agent.
func (m *Manager) pingAgents(ctx context.Context) {
	agents, err := m.store.ListAgents(ctx)
	if err != nil {
		return
	}
	for _, a := range agents {
		var info map[string]any
		err := m.AgentCall(ctx, a.ID, http.MethodGet, "/agent/v1/info", nil, &info)
		status, msg := "online", ""
		if err != nil {
			status, msg = "offline", err.Error()
		}
		_ = m.store.SetAgentStatus(ctx, db.SetAgentStatusParams{Status: status, LastError: msg, Column3: status, ID: a.ID})
		if a.Status != status {
			m.hub.Publish("agent.status", map[string]any{"id": a.ID, "status": status, "lastError": msg})
		}
	}
}
