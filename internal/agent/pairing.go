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

// consume checks input against the current code in constant time; a match
// uses the code up (a new one is generated).
func (p *pairing) consume(input string) bool {
	in := normalizePairCode(input)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.code == "" || !p.now().Before(p.expires) {
		p.rotateLocked()
	}
	if len(in) != pairCodeLen || subtle.ConstantTimeCompare([]byte(in), []byte(p.code)) != 1 {
		return false
	}
	p.rotateLocked()
	return true
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
	id  string
	sum [sha256.Size]byte
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
	var req struct {
		Code       string `json:"code"`
		ServerName string `json:"serverName"`
		ServerID   string `json:"serverId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if !validServerID(req.ServerID) {
		writeError(w, http.StatusBadRequest, "invalid_server_id", "serverId invalide")
		return
	}
	if !s.pairing.consume(req.Code) {
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
	sv := pairedServer{ID: id, Name: name, TokenHash: hex.EncodeToString(sum[:]), PairedAt: time.Now().UTC().Truncate(time.Second)}

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

	s.log.Info("serveur appairé", "serveur", name, "id", id, "ip", clientIP(r))
	writeJSON(w, pairResponse{Token: tok, UID: s.uid, Name: s.name, Version: s.version})
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
