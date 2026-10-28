package sources

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/fs0ciety7000/aether-media/internal/agentproto"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
	"github.com/fs0ciety7000/aether-media/internal/vfs/agentfs"
)

// Agent status values beyond online/offline.
const (
	// AgentCertMismatch: the agent presents another certificate than the
	// pinned one; every call is refused until it is paired again.
	AgentCertMismatch = "cert_mismatch"
)

// Transport describes how the server reaches an agent.
const (
	TransportPinned = "https-pinned" // HTTPS, certificate pinned by fingerprint
	TransportCA     = "https-ca"     // HTTPS, certificate checked by the system CAs
	TransportHTTP   = "http"         // plain HTTP (paired before HTTPS, or tls = false)
)

// AgentTransport classifies a paired agent row.
func AgentTransport(a db.RemoteAgent) string {
	switch {
	case a.Fingerprint != "":
		return TransportPinned
	case strings.HasPrefix(strings.ToLower(a.Url), "https://"):
		return TransportCA
	}
	return TransportHTTP
}

// agentCreds resolves the URL, certificate pin and token of an agent
// source: from the paired agent when AgentID is set, else from the source
// itself (manual token).
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
	cfg.Fingerprint = a.Fingerprint
	return cfg, asec, nil
}

// PairResult is what an agent returns when a pairing proof is accepted.
type PairResult struct {
	Token       string `json:"token"`
	UID         string `json:"uid"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	PairVersion int    `json:"pairVersion"`
	Fingerprint string `json:"fingerprint"`
	CertSource  string `json:"certSource"`
	Proof       string `json:"proof"`
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

// agentCall performs an agent API request with c and decodes the JSON
// response.
func agentCall(ctx context.Context, c *http.Client, method, base, path, token string, body, out any) error {
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
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("agent injoignable : %w", err)
	}
	defer drainClose(res)
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

// normalizeCode accepts the code with or without dash/spaces, in any case.
func normalizeCode(in string) string {
	var b strings.Builder
	for _, r := range in {
		if r == '-' || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// pairExchange runs one v2 pairing exchange against base. It returns the
// agent's answer and what the server saw of the TLS certificate. The code
// never travels: the request carries HMAC(Argon2id(code, nonce), seen
// fingerprint…), and the answer must carry the agent's own proof.
// seen, when set, is a handshake just made with base (no second one).
func (m *Manager) pairExchange(ctx context.Context, base, code string, seen *agentfs.TLSInfo) (PairResult, agentfs.TLSInfo, error) {
	var tlsInfo agentfs.TLSInfo
	client := agentfs.ClientFor(base, "")
	if strings.HasPrefix(base, "https://") {
		var err error
		if seen != nil {
			tlsInfo = *seen
		} else if tlsInfo, err = agentfs.ProbeTLS(ctx, base); err != nil {
			return PairResult{}, tlsInfo, fmt.Errorf("%w: agent injoignable: %v", vfs.ErrOffline, err) //nolint:errorlint // keep a single sentinel
		}
		// The request goes over a connection pinned to the certificate just
		// seen, the one the proof is bound to.
		client = agentfs.ClientFor(base, tlsInfo.Fingerprint)
	}
	nonce := make([]byte, agentproto.NonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return PairResult{}, tlsInfo, err
	}
	key := agentproto.PairKey(code, nonce)
	var res PairResult
	err := agentCall(ctx, client, http.MethodPost, base, "/agent/v1/pair", "", map[string]any{
		"version":     agentproto.PairVersion,
		"serverName":  m.ServerName,
		"serverId":    m.ServerID,
		"fingerprint": tlsInfo.Fingerprint,
		"nonce":       base64.RawURLEncoding.EncodeToString(nonce),
		"proof":       base64.RawURLEncoding.EncodeToString(agentproto.ServerProof(key, tlsInfo.Fingerprint, nonce, m.ServerID)),
	}, &res)
	if err != nil {
		return res, tlsInfo, err
	}
	proof, _ := base64.RawURLEncoding.DecodeString(res.Proof)
	if res.PairVersion < agentproto.PairVersion || res.Token == "" || res.UID == "" ||
		!agentproto.Equal(proof, agentproto.AgentProof(key, tlsInfo.Fingerprint, nonce, res.Token)) {
		return res, tlsInfo, &ValidationError{Code: "agent_proof_invalid", Msg: "réponse d'appairage non authentique : l'agent n'a pas prouvé connaître le code (interception possible)"}
	}
	return res, tlsInfo, nil
}

// Pair exchanges a pairing code displayed by an agent for a token and saves
// the agent. Re-pairing an agent (same uid, or agentID given: e.g. moving an
// agent paired over HTTP to HTTPS) updates it. An http:// address of an
// agent that requires HTTPS is upgraded to https:// automatically.
func (m *Manager) Pair(ctx context.Context, rawURL, code string, agentID int64) (db.RemoteAgent, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return db.RemoteAgent{}, invalid("adresse de l'agent invalide (ex. https://192.168.1.20:8097)")
	}
	u.RawQuery, u.Fragment = "", ""
	base := strings.TrimSuffix(u.String(), "/")
	code = normalizeCode(code)
	if len(code) != 8 {
		return db.RemoteAgent{}, invalid("code d'appairage invalide (8 caractères, ex. K7PQ-4XRM)")
	}
	var prev *db.RemoteAgent
	if agentID != 0 {
		a, err := m.store.GetAgent(ctx, agentID)
		if errors.Is(err, sql.ErrNoRows) {
			return db.RemoteAgent{}, invalid("agent à réappairer introuvable")
		}
		if err != nil {
			return db.RemoteAgent{}, err
		}
		prev = &a
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var seen *agentfs.TLSInfo
	if u.Scheme == "http" {
		// Agents serve HTTPS and HTTP on the same port: prefer HTTPS before
		// sending anything, so that the proof never travels in clear.
		hu := *u
		hu.Scheme = "https"
		pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
		if ti, err := agentfs.ProbeTLS(pctx, hu.String()); err == nil {
			u.Scheme = "https"
			base = strings.TrimSuffix(u.String(), "/")
			seen = &ti
		}
		pcancel()
	}
	res, tlsInfo, err := m.pairExchange(ctx, base, code, seen)
	var ae *AgentError
	if errors.As(err, &ae) && ae.Code == "tls_required" && u.Scheme == "http" {
		u.Scheme = "https"
		base = strings.TrimSuffix(u.String(), "/")
		res, tlsInfo, err = m.pairExchange(ctx, base, code, nil)
	}
	if err != nil {
		return db.RemoteAgent{}, pairError(err)
	}

	fp := ""
	switch {
	case res.Fingerprint != "":
		// The agent terminates TLS itself and checked that we saw its
		// certificate. A certificate from a recognised CA (tls_cert) is
		// verified like any website, so that its renewals do not break the
		// pairing; anything else is pinned.
		if got, _ := agentproto.NormalizeFingerprint(res.Fingerprint); got != tlsInfo.Fingerprint {
			return db.RemoteAgent{}, &ValidationError{Code: "agent_fingerprint_mismatch", Msg: "empreinte annoncée par l'agent différente du certificat reçu (interception possible)"}
		}
		if res.CertSource != "custom" || !tlsInfo.CAValid {
			fp = tlsInfo.Fingerprint
		}
	case u.Scheme == "https":
		// Agent with tls = false behind an HTTPS reverse proxy: it cannot
		// check which certificate we saw, so only a CA-verified one is safe.
		if !tlsInfo.CAValid {
			_ = agentCall(ctx, agentfs.ClientFor(base, tlsInfo.Fingerprint), http.MethodDelete, base, "/agent/v1/pair", res.Token, nil, nil)
			return db.RemoteAgent{}, &ValidationError{Code: "agent_cert_untrusted", Msg: "l'agent n'assure pas lui-même HTTPS (tls = false) et le certificat du proxy n'est pas reconnu : utilisez un certificat d'une autorité reconnue sur le proxy, ou activez HTTPS sur l'agent"}
		}
	default:
		m.log.Warn("agent appairé en HTTP non chiffré (tls = false) : réservez cet usage à un VPN ou à un réseau de confiance", "url", base)
	}

	sealed, err := m.box.SealJSON(Secrets{Token: res.Token})
	if err != nil {
		return db.RemoteAgent{}, err
	}
	var a db.RemoteAgent
	if prev != nil && prev.Uid != res.UID {
		if !strings.HasPrefix(prev.Uid, "legacy:") {
			_ = agentCall(ctx, agentfs.ClientFor(base, fp), http.MethodDelete, base, "/agent/v1/pair", res.Token, nil, nil)
			return db.RemoteAgent{}, &ValidationError{Code: "agent_not_same", Msg: "la machine qui a répondu n'est pas cet agent (identifiant différent)"}
		}
		if other, err := m.store.GetAgentByUID(ctx, res.UID); err == nil && other.ID != prev.ID {
			return db.RemoteAgent{}, &ValidationError{Code: "agent_already_paired", Msg: fmt.Sprintf("cet agent est déjà appairé (« %s »)", other.Name)}
		}
	}
	if prev != nil {
		a, err = m.store.RepairAgent(ctx, db.RepairAgentParams{Uid: res.UID, Name: res.Name, Url: base, Secret: sealed, Version: res.Version, Fingerprint: fp, ID: prev.ID})
	} else {
		a, err = m.store.UpsertAgent(ctx, db.UpsertAgentParams{Uid: res.UID, Name: res.Name, Url: base, Secret: sealed, Version: res.Version, Fingerprint: fp})
	}
	if err != nil {
		return a, err
	}
	m.gate(AgentConfig{AgentID: a.ID, URL: base, Fingerprint: fp}, Secrets{Token: res.Token}).up()
	m.log.Info("agent appairé", "agent", a.Name, "url", base, "transport", AgentTransport(a), "empreinte", agentproto.FormatFingerprint(fp))
	m.forgetAgentSources(ctx, a.ID) // new token / certificate: reopen file systems
	if m.hub != nil {
		m.hub.PublishAdmin("agent.status", map[string]any{"id": a.ID, "status": a.Status})
	}
	return a, nil
}

// pairError maps pairing failures to user-facing errors.
func pairError(err error) error {
	var ae *AgentError
	if errors.As(err, &ae) {
		switch {
		case ae.Code == "fingerprint_mismatch":
			return &ValidationError{Code: "agent_fingerprint_mismatch", Msg: "le certificat reçu n'est pas celui de l'agent : une interception (intermédiaire) est possible. L'agent a invalidé le code ; vérifiez l'adresse et le réseau, puis réessayez avec le nouveau code."}
		case ae.Code == "pairing_upgrade_required":
			return &ValidationError{Code: "agent_too_old", Msg: ae.Msg}
		case ae.Status == http.StatusUnauthorized:
			return invalid("code d'appairage incorrect ou expiré (ou agent trop ancien : mettez-le à jour)")
		case ae.Status == http.StatusTooManyRequests:
			return invalid("trop d'essais : patientez quelques minutes")
		case ae.Status == http.StatusBadRequest, ae.Status == http.StatusForbidden:
			return invalid("appairage refusé par l'agent : %s", ae.Error())
		}
		return err
	}
	var ve *ValidationError
	if errors.As(err, &ve) || errors.Is(err, vfs.ErrOffline) {
		return err
	}
	return fmt.Errorf("%w: agent injoignable: %v", vfs.ErrOffline, err) //nolint:errorlint // keep a single sentinel
}

// AgentCall runs an authenticated request against a paired agent, over its
// pinned transport.
func (m *Manager) AgentCall(ctx context.Context, agentID int64, method, path string, body, out any) error {
	a, err := m.store.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	var sec Secrets
	if err := m.box.OpenJSON(a.Secret, &sec); err != nil {
		return err
	}
	if a.Fingerprint != "" && !strings.HasPrefix(a.Url, "https://") {
		return errors.New("agent épinglé joint en http:// : réappairez-le")
	}
	g := m.gate(AgentConfig{AgentID: a.ID, URL: a.Url, Fingerprint: a.Fingerprint}, sec)
	if err := g.check(); err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return g.done(ctx, agentCall(cctx, agentfs.ClientFor(a.Url, a.Fingerprint), method, a.Url, path, sec.Token, body, out))
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

// agentStatus maps a call error to an agent status.
func agentStatus(err error) string {
	switch {
	case err == nil:
		return "online"
	case errors.Is(err, agentfs.ErrFingerprintMismatch):
		return AgentCertMismatch
	}
	return "offline"
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
		status, msg := agentStatus(err), ""
		if err != nil {
			msg = err.Error()
			var fe *agentfs.FingerprintError
			if errors.As(err, &fe) {
				msg = fe.Error()
			}
		}
		_ = m.store.SetAgentStatus(ctx, db.SetAgentStatusParams{Status: status, LastError: msg, Column3: status, ID: a.ID})
		if a.Status != status {
			if status == AgentCertMismatch {
				m.log.Error("agent : certificat différent de celui épinglé, appels refusés", "agent", a.Name, "url", a.Url, "err", msg)
			}
			m.hub.PublishAdmin("agent.status", map[string]any{"id": a.ID, "status": status, "lastError": msg})
		}
	}
}
