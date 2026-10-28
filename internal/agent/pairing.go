package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fs0ciety7000/aether-media/internal/agentproto"
)

const (
	pairAlphabet  = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	pairCodeLen   = 8
	pairCodeTTL   = 10 * time.Minute
	pairMaxFails  = 5
	pairFailSpan  = 10 * time.Minute
	maxServerID   = 128
	maxServerName = 100
)

// pairing holds the current one-time pairing code.
type pairing struct {
	mu      sync.Mutex
	code    string // 8 characters, no dash
	expires time.Time
	now     func() time.Time
	log     *slog.Logger
}

func newPairing(log *slog.Logger) *pairing {
	return &pairing{now: time.Now, log: log}
}

func genPairCode() string {
	const n = byte(len(pairAlphabet))
	const limit = 256 - 256%int(n) // rejection sampling: no modulo bias
	out := make([]byte, 0, pairCodeLen)
	var buf [16]byte
	for len(out) < pairCodeLen {
		if _, err := rand.Read(buf[:]); err != nil {
			panic(err) // crypto/rand never fails on supported platforms
		}
		for _, b := range buf {
			if int(b) < limit && len(out) < pairCodeLen {
				out = append(out, pairAlphabet[b%n])
			}
		}
	}
	return string(out)
}

// formatPairCode renders a code as XXXX-XXXX.
func formatPairCode(c string) string {
	if len(c) != pairCodeLen {
		return c
	}
	return c[:4] + "-" + c[4:]
}

// normalizePairCode accepts the code with or without dash/spaces, in any case.
func normalizePairCode(in string) string {
	var b strings.Builder
	for _, r := range in {
		if r == '-' || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

func (p *pairing) rotateLocked() {
	p.code = genPairCode()
	p.expires = p.now().Add(pairCodeTTL)
	p.log.Info("Code d'appairage : " + formatPairCode(p.code) + " (valable 10 min)")
}

// current returns the code (formatted XXXX-XXXX) and its expiry, renewing
// it first when expired.
func (p *pairing) current() (string, time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.code == "" || !p.now().Before(p.expires) {
		p.rotateLocked()
	}
	return formatPairCode(p.code), p.expires
}

// snapshot returns the current raw code (renewed first when expired).
func (p *pairing) snapshot() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.code == "" || !p.now().Before(p.expires) {
		p.rotateLocked()
	}
	return p.code
}

// consumeIf uses code up when it is still the current, unexpired one.
func (p *pairing) consumeIf(code string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.code == "" || !p.now().Before(p.expires) || subtle.ConstantTimeCompare([]byte(code), []byte(p.code)) != 1 {
		return false
	}
	p.rotateLocked()
	return true
}

// invalidate replaces the current code (it may have been exposed).
func (p *pairing) invalidate(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.log.Warn("code d'appairage invalidé : " + reason)
	p.rotateLocked()
}

// rotateLoop logs each new code when the previous one expires.
func (p *pairing) rotateLoop(ctx context.Context) {
	for {
		_, exp := p.current()
		t := time.NewTimer(time.Until(exp) + 10*time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// PairingCode returns the current pairing code (XXXX-XXXX) and its expiry.
func (s *Server) PairingCode() (string, time.Time) { return s.pairing.current() }

// limiterKey groups IPv6 clients by /64 so that address rotation does not
// multiply the pairing attempts.
func limiterKey(ip string) string {
	a := net.ParseIP(ip)
	if a == nil || a.To4() != nil {
		return ip
	}
	return a.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// ---- server token sums ----

type serverSum struct {
	id    string
	sum   [sha256.Size]byte
	plain bool
}

func (s *Server) rebuildSumsLocked() {
	s.serverSums = s.serverSums[:0]
	for _, sv := range s.state.Servers {
		h, err := hex.DecodeString(sv.TokenHash)
		if err != nil || len(h) != sha256.Size {
			continue
		}
		var ss serverSum
		ss.id = sv.ID
		ss.plain = sv.Plain
		copy(ss.sum[:], h)
		s.serverSums = append(s.serverSums, ss)
	}
}

// matchToken compares sum with the legacy token and every paired server's
// token, all in constant time (no early exit).
func (s *Server) matchToken(sum [sha256.Size]byte) (c caller, ok bool) {
	legacy := subtle.ConstantTimeCompare(sum[:], s.tokenSum[:])
	s.mu.RLock()
	defer s.mu.RUnlock()
	found := 0
	for i := range s.serverSums {
		if subtle.ConstantTimeCompare(sum[:], s.serverSums[i].sum[:]) == 1 {
			c.serverID = s.serverSums[i].id
			c.plain = s.serverSums[i].plain
			found = 1
		}
	}
	if found == 1 {
		return c, true
	}
	if legacy == 1 {
		return caller{legacy: true}, true
	}
	return caller{}, false
}

type caller struct {
	legacy   bool
	serverID string
	plain    bool // paired over plain HTTP (compatibility)
}

type callerKey struct{}

func callerFrom(ctx context.Context) caller {
	c, _ := ctx.Value(callerKey{}).(caller)
	return c
}

// ---- HTTP: /pair ----

type pairResponse struct {
	Token   string `json:"token"`
	UID     string `json:"uid"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// Pairing v2: the agent's certificate fingerprint ("" with tls =
	// false), its origin and the agent's proof of knowledge of the code.
	PairVersion int    `json:"pairVersion"`
	Fingerprint string `json:"fingerprint"`
	CertSource  string `json:"certSource,omitempty"`
	Proof       string `json:"proof"`
}

// pairRequest is the body of POST /agent/v1/pair (v2). The code itself never
// travels: the server proves it knows it with an HMAC keyed by
// Argon2id(code, nonce) over the certificate fingerprint it saw.
type pairRequest struct {
	Version     int    `json:"version"`
	Code        string `json:"code"` // v1 (code in clear): refused
	ServerName  string `json:"serverName"`
	ServerID    string `json:"serverId"`
	Fingerprint string `json:"fingerprint"`
	Nonce       string `json:"nonce"` // base64url, 16 bytes
	Proof       string `json:"proof"` // base64url
}

func validServerID(id string) bool {
	if len(id) > maxServerID {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == ':':
		default:
			return false
		}
	}
	return true
}

// cleanDisplayName drops control characters and bounds the length.
func cleanDisplayName(n string, maxRunes int) string {
	n = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(n))
	if r := []rune(n); len(r) > maxRunes {
		n = string(r[:maxRunes])
	}
	return n
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	key := limiterKey(clientIP(r))
	if s.pairLimiter.blocked(key) {
		w.Header().Set("Retry-After", "600")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "trop d'essais de code d'appairage")
		return
	}
	// Requiring JSON forces a CORS preflight: a web page cannot blindly
	// submit guesses from a browser on the LAN.
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "application/json attendu")
		return
	}
	var req pairRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Version < agentproto.PairVersion || req.Proof == "" {
		if req.Code != "" {
			s.pairing.invalidate("reçu en clair d'un serveur Aether trop ancien")
		}
		writeError(w, http.StatusBadRequest, "pairing_upgrade_required", "appairage v1 (code en clair) refusé : mettez à jour le serveur Aether")
		return
	}
	if s.tls != nil && r.TLS == nil {
		writeError(w, http.StatusUpgradeRequired, "tls_required", "appairage en HTTPS uniquement : utilisez https://")
		return
	}
	if !validServerID(req.ServerID) {
		writeError(w, http.StatusBadRequest, "invalid_server_id", "serverId invalide")
		return
	}
	nonce, err1 := base64.RawURLEncoding.DecodeString(req.Nonce)
	proof, err2 := base64.RawURLEncoding.DecodeString(req.Proof)
	if err1 != nil || err2 != nil || len(nonce) != agentproto.NonceLen || len(proof) != sha256.Size {
		writeError(w, http.StatusBadRequest, "invalid_body", "nonce ou preuve d'appairage invalide")
		return
	}
	seen, err := agentproto.NormalizeFingerprint(req.Fingerprint)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "empreinte invalide")
		return
	}
	// The fingerprint the proof is bound to. When the agent terminates TLS
	// itself it must be its own: anything else means the server talked to
	// someone else's certificate (interception), and the code, which the
	// recorded proof could be attacked with, is thrown away.
	bound := seen
	if s.tls != nil {
		if seen != s.tls.fingerprint {
			s.pairing.invalidate("le serveur a vu un autre certificat (empreinte " + agentproto.FormatFingerprint(seen) + ") : intermédiaire possible")
			s.pairLimiter.fail(key)
			writeError(w, http.StatusConflict, "fingerprint_mismatch", "le serveur a vu un autre certificat que celui de l'agent (interception possible) ; le code a été invalidé")
			return
		}
		bound = s.tls.fingerprint
	}
	code := s.pairing.snapshot()
	select {
	case s.pairKDF <- struct{}{}:
	case <-r.Context().Done():
		return
	}
	pairKey := agentproto.PairKey(code, nonce)
	<-s.pairKDF
	if !agentproto.Equal(proof, agentproto.ServerProof(pairKey, bound, nonce, req.ServerID)) || !s.pairing.consumeIf(code) {
		if s.pairLimiter.fail(key) {
			s.log.Warn("trop de codes d'appairage erronés, adresse bloquée 10 min", "ip", clientIP(r))
		}
		writeError(w, http.StatusUnauthorized, "invalid_code", "code d'appairage invalide ou expiré")
		return
	}
	id := req.ServerID
	if id == "" {
		uid, err := newUID()
		if err != nil {
			s.writeStatusError(w, err)
			return
		}
		id = uid
	}
	name := cleanDisplayName(req.ServerName, maxServerName)
	if name == "" {
		name = "Serveur Aether"
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		s.writeStatusError(w, err)
		return
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(tok))
	sv := pairedServer{ID: id, Name: name, TokenHash: hex.EncodeToString(sum[:]), PairedAt: time.Now().UTC().Truncate(time.Second), Plain: r.TLS == nil}

	s.mu.Lock()
	prev := s.state.Servers
	next := make([]pairedServer, 0, len(prev)+1)
	for _, o := range prev {
		if o.ID != id { // pairing again replaces (revokes) the previous token
			next = append(next, o)
		}
	}
	s.state.Servers = append(next, sv)
	if err := s.saveLocked(); err != nil {
		s.state.Servers = prev
		s.mu.Unlock()
		s.writeStatusError(w, err)
		return
	}
	s.rebuildSumsLocked()
	s.mu.Unlock()

	s.plainWarned.Delete("server:" + id)
	s.log.Info("serveur appairé", "serveur", name, "id", id, "ip", clientIP(r), "https", r.TLS != nil)
	resp := pairResponse{
		Token: tok, UID: s.uid, Name: s.name, Version: s.version, PairVersion: agentproto.PairVersion,
		Fingerprint: s.Fingerprint(),
		Proof:       base64.RawURLEncoding.EncodeToString(agentproto.AgentProof(pairKey, bound, nonce, tok)),
	}
	if s.tls != nil {
		resp.CertSource = s.tls.source
	}
	writeJSON(w, resp)
}

func (s *Server) handleUnpair(w http.ResponseWriter, r *http.Request) {
	c := callerFrom(r.Context())
	if c.serverID == "" {
		writeError(w, http.StatusConflict, "legacy_token", "le jeton agent.token ne peut pas être révoqué (supprimez le fichier)")
		return
	}
	s.mu.Lock()
	prev := s.state.Servers
	next := make([]pairedServer, 0, len(prev))
	for _, o := range prev {
		if o.ID != c.serverID {
			next = append(next, o)
		}
	}
	s.state.Servers = next
	if err := s.saveLocked(); err != nil {
		s.state.Servers = prev
		s.mu.Unlock()
		s.writeStatusError(w, err)
		return
	}
	s.rebuildSumsLocked()
	s.mu.Unlock()
	s.log.Info("serveur désappairé", "id", c.serverID)
	w.WriteHeader(http.StatusNoContent)
}
