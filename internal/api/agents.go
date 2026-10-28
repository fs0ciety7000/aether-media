package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agent"
	"github.com/fs0ciety7000/aether-media/internal/fsbrowse"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/vfs/agentfs"
)

// DiscoveryPort is the UDP port agents answer discovery on.
const DiscoveryPort = 7360

type agentDTO struct {
	ID        int64      `json:"id"`
	UID       string     `json:"uid"`
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	Version   string     `json:"version,omitempty"`
	Status    string     `json:"status"`
	LastError string     `json:"lastError,omitempty"`
	LastSeen  *time.Time `json:"lastSeen,omitempty"`
	Sources   int64      `json:"sources"`
	// Fingerprint: SHA-256 (lowercase hex) of the pinned certificate;
	// Transport: https-pinned | https-ca | http.
	Fingerprint string `json:"fingerprint,omitempty"`
	Transport   string `json:"transport"`
}

func (s *Server) agentDTO(r *http.Request, a db.RemoteAgent) agentDTO {
	dto := agentDTO{ID: a.ID, UID: a.Uid, Name: a.Name, URL: a.Url, Version: a.Version, Status: a.Status, LastError: a.LastError,
		Fingerprint: a.Fingerprint, Transport: sources.AgentTransport(a)}
	if a.LastSeen.Valid {
		t := time.Unix(a.LastSeen.Int64, 0).UTC()
		dto.LastSeen = &t
	}
	dto.Sources, _ = s.store.CountAgentSources(r.Context(), a.ID)
	return dto
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.ListAgents(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := make([]agentDTO, 0, len(agents))
	for _, a := range agents {
		out = append(out, s.agentDTO(r, a))
	}
	writeJSON(w, http.StatusOK, out)
}

type discoveredDTO struct {
	agent.Discovered
	Paired  bool   `json:"paired"`
	AgentID *int64 `json:"agentId,omitempty"`
}

// discoverAgents: POST /api/v1/agents/discover — broadcasts on the LAN.
func (s *Server) discoverAgents(w http.ResponseWriter, r *http.Request) {
	found, err := agent.Discover(r.Context(), DiscoveryPort, 2*time.Second)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "discovery_failed", "recherche sur le réseau impossible : "+err.Error())
		return
	}
	known, _ := s.store.ListAgents(r.Context())
	out := []discoveredDTO{}
	for _, d := range found {
		dto := discoveredDTO{Discovered: d}
		for _, k := range known {
			if k.Uid == d.UID {
				id := k.ID
				dto.Paired, dto.AgentID = true, &id
			}
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

type pairRequest struct {
	URL  string `json:"url"`
	Code string `json:"code"`
	// AgentID re-pairs that agent (new token and certificate, e.g. moving
	// from HTTP to HTTPS, or after a certificate change).
	AgentID int64 `json:"agentId,omitempty"`
}

// pairAgent: POST /api/v1/agents/pair — exchanges the code shown by the agent.
func (s *Server) pairAgent(w http.ResponseWriter, r *http.Request) {
	if !s.requireSources(w) {
		return
	}
	var req pairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	a, err := s.Sources.Pair(r.Context(), req.URL, req.Code, req.AgentID)
	if s.sourceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, s.agentDTO(r, a))
}

func (s *Server) loadAgent(w http.ResponseWriter, r *http.Request) (db.RemoteAgent, bool) {
	id, err := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "identifiant invalide")
		return db.RemoteAgent{}, false
	}
	a, err := s.store.GetAgent(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "agent introuvable")
		return a, false
	}
	if err != nil {
		s.internalError(w, err)
		return a, false
	}
	return a, s.requireSources(w)
}

// agentProxyError maps agent call failures to API errors.
func (s *Server) agentProxyError(w http.ResponseWriter, err error) {
	if errors.Is(err, agentfs.ErrFingerprintMismatch) {
		writeError(w, http.StatusBadGateway, "agent_cert_mismatch", err.Error())
		return
	}
	var ae *sources.AgentError
	if errors.As(err, &ae) {
		code := ae.Code
		if code == "" {
			code = "agent_error"
		}
		status := http.StatusBadGateway
		switch ae.Status {
		case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict:
			status = ae.Status
		case http.StatusUnauthorized:
			code = "agent_unpaired"
		case http.StatusUpgradeRequired:
			code = "agent_tls_required"
		}
		writeError(w, status, code, ae.Error())
		return
	}
	writeError(w, http.StatusBadGateway, "agent_offline", err.Error())
}

// agentBrowse: GET /api/v1/agents/{aid}/browse?path= — folders on the agent machine.
func (s *Server) agentBrowse(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var out json.RawMessage
	q := url.Values{}
	if p := r.URL.Query().Get("path"); p != "" {
		q.Set("path", p)
	}
	if err := s.Sources.AgentCall(r.Context(), a.ID, http.MethodGet, "/agent/v1/browse?"+q.Encode(), nil, &out); err != nil {
		s.agentProxyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// agentShares: GET/POST /api/v1/agents/{aid}/shares
func (s *Server) agentShares(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var out json.RawMessage
	var body any
	status := http.StatusOK
	if r.Method == http.MethodPost {
		var req struct {
			Path string `json:"path"`
			Name string `json:"name,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil || req.Path == "" {
			writeError(w, http.StatusBadRequest, "invalid_json", "dossier requis")
			return
		}
		body, status = req, http.StatusCreated
	}
	if err := s.Sources.AgentCall(r.Context(), a.ID, r.Method, "/agent/v1/shares", body, &out); err != nil {
		s.agentProxyError(w, err)
		return
	}
	writeJSON(w, status, out)
}

// deleteAgent: DELETE /api/v1/agents/{aid} — unpairs (refused while sources use it).
func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	if n, _ := s.store.CountAgentSources(r.Context(), a.ID); n > 0 {
		writeError(w, http.StatusConflict, "agent_in_use", "supprimez d'abord les sources qui utilisent cet agent")
		return
	}
	if err := s.Sources.Unpair(r.Context(), a.ID); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// browseLocal: GET /api/v1/fs/browse?path= — folders on the server itself
// (picker for local sources).
func (s *Server) browseLocal(w http.ResponseWriter, r *http.Request) {
	roots := s.BrowseRoots
	if roots == nil {
		roots = fsbrowse.DefaultRoots()
	}
	p := r.URL.Query().Get("path")
	dirs, err := fsbrowse.List(roots, p)
	if errors.Is(err, fsbrowse.ErrDenied) {
		writeError(w, http.StatusForbidden, "denied", "dossier hors des emplacements autorisés")
		return
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "dossier illisible")
		return
	}
	out := map[string]any{"path": p, "dirs": dirs}
	if p != "" {
		if resolved, ok := fsbrowse.Allowed(roots, p); ok {
			for _, root := range roots {
				if resolved != root {
					if parent := parentWithin(roots, resolved); parent != "" {
						out["parent"] = parent
					}
					break
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// parentWithin returns p's parent when it is still inside the roots.
func parentWithin(roots []string, p string) string {
	for _, r := range roots {
		if p == r {
			return ""
		}
	}
	parent := filepathDir(p)
	if _, ok := fsbrowse.Allowed(roots, parent); ok {
		return parent
	}
	return ""
}

func filepathDir(p string) string { return filepath.Dir(p) }
