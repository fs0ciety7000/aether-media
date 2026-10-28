// Package agentproto holds what the Aether agent and the server must agree
// on byte for byte: the TLS certificate fingerprint format and the pairing
// proofs that bind a pairing code to the certificate the server saw
// (docs/AGENT_PROTOCOL.md, « Appairage lié à l'empreinte »).
package agentproto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/argon2"
)

// PairVersion is the version of the pairing exchange (POST /agent/v1/pair).
const PairVersion = 2

// Argon2id cost of the pairing key. Deriving it costs ~0.1 s and 32 MiB, so
// an attacker who recorded one exchange cannot test the 31^8 possible codes
// within their 10-minute lifetime (see the threat model).
const (
	kdfTime    = 3
	kdfMemory  = 32 * 1024 // KiB
	kdfThreads = 1
	keyLen     = 32
	// NonceLen is the size of the server's random pairing nonce.
	NonceLen = 16
)

// Fingerprint is the lowercase hex SHA-256 of a DER certificate.
func Fingerprint(der []byte) string {
	s := sha256.Sum256(der)
	return hex.EncodeToString(s[:])
}

// CertFingerprint is Fingerprint of a parsed certificate.
func CertFingerprint(c *x509.Certificate) string { return Fingerprint(c.Raw) }

// ErrBadFingerprint reports a fingerprint that is not 32 hex bytes.
var ErrBadFingerprint = errors.New("empreinte invalide : 64 caractères hexadécimaux attendus (SHA-256)")

// NormalizeFingerprint accepts a fingerprint in any usual form (groups,
// colons, case, "sha256:" prefix) and returns the canonical lowercase hex.
// An empty input stays empty.
func NormalizeFingerprint(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" {
		return "", nil
	}
	if p, rest, ok := strings.Cut(s, ":"); ok && strings.EqualFold(p, "sha256") {
		s = rest
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ':' || r == ' ' || r == '-' || r == '\t':
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
			b.WriteRune(r)
		case r >= 'A' && r <= 'F':
			b.WriteRune(r + ('a' - 'A'))
		default:
			return "", ErrBadFingerprint
		}
	}
	if b.Len() != 2*sha256.Size {
		return "", ErrBadFingerprint
	}
	return b.String(), nil
}

// FormatFingerprint renders a fingerprint for humans: 16 groups of 4
// uppercase hex digits ("3F2A 9C01 …"). Invalid input is returned as is.
func FormatFingerprint(fp string) string {
	n, err := NormalizeFingerprint(fp)
	if err != nil || n == "" {
		return fp
	}
	n = strings.ToUpper(n)
	var b strings.Builder
	for i := 0; i < len(n); i += 4 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(n[i : i+4])
	}
	return b.String()
}

// PairKey derives the pairing key from the normalised code (8 characters,
// no dash, uppercase) and the server's nonce. Memory-hard, so that a
// recorded exchange does not allow testing codes cheaply.
func PairKey(code string, nonce []byte) []byte {
	salt := append([]byte("aether-pair-v2|"), nonce...)
	return argon2.IDKey([]byte(code), salt, kdfTime, kdfMemory, kdfThreads, keyLen)
}

func mac(key []byte, parts ...string) []byte {
	h := hmac.New(sha256.New, key)
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return h.Sum(nil)
}

// ServerProof is sent by the server: it proves knowledge of the code and
// binds it to the certificate fingerprint the server saw ("" over plain
// HTTP) and to the server's identity.
func ServerProof(key []byte, fingerprint string, nonce []byte, serverID string) []byte {
	return mac(key, "aether-pair-v2", "server", fingerprint, hex.EncodeToString(nonce), serverID)
}

// AgentProof is returned by the agent: it proves the agent knows the code
// too (a fake agent cannot complete the pairing) and binds the issued token.
func AgentProof(key []byte, fingerprint string, nonce []byte, token string) []byte {
	return mac(key, "aether-pair-v2", "agent", fingerprint, hex.EncodeToString(nonce), token)
}

// Equal compares two proofs in constant time.
func Equal(a, b []byte) bool { return hmac.Equal(a, b) }
