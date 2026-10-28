package agentfs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agentproto"
)

// ErrFingerprintMismatch is matched (errors.Is) by a FingerprintError.
var ErrFingerprintMismatch = errors.New("agentfs: certificate fingerprint mismatch")

// FingerprintError reports an agent presenting another certificate than the
// pinned one: interception, or a regenerated certificate (pair again).
type FingerprintError struct {
	Want, Got string // lowercase hex SHA-256
}

func (e *FingerprintError) Error() string {
	return fmt.Sprintf("certificat de l'agent inattendu (empreinte %s au lieu de %s) : interception possible ; si le certificat a été régénéré, réappairez l'agent",
		short(e.Got), short(e.Want))
}

func (e *FingerprintError) Is(target error) bool { return target == ErrFingerprintMismatch }

func short(fp string) string {
	f := agentproto.FormatFingerprint(fp)
	if len(f) > 19 {
		return f[:19] + "…"
	}
	return f
}

// PinnedTLS returns a client TLS configuration that accepts exactly the
// certificate whose SHA-256 is fingerprint: no CA, no host name check.
// VerifyConnection (unlike VerifyPeerCertificate) also runs on resumed
// sessions, so a resumed connection cannot skip the check.
func PinnedTLS(fingerprint string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // replaced by the fingerprint check below
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("agentfs: no certificate presented")
			}
			if got := agentproto.CertFingerprint(cs.PeerCertificates[0]); got != fingerprint {
				return &FingerprintError{Want: fingerprint, Got: got}
			}
			return nil
		},
	}
}

var clients sync.Map // fingerprint ("" = CA verification) → *http.Client

// ClientFor returns the shared HTTP client for agents pinned to fingerprint
// ("" verifies certificates against the system CAs, as for any HTTPS site,
// and is also used for plain HTTP agents). No overall timeout: bodies are
// long media streams; callers bound metadata calls with contexts.
func ClientFor(fingerprint string) *http.Client {
	if c, ok := clients.Load(fingerprint); ok {
		return c.(*http.Client)
	}
	c, _ := clients.LoadOrStore(fingerprint, PinClient(nil, fingerprint))
	return c.(*http.Client)
}

// PinClient returns a copy of base (DefaultClient when nil) whose transport
// pins fingerprint. An empty fingerprint returns base unchanged.
func PinClient(base *http.Client, fingerprint string) *http.Client {
	if base == nil {
		base = DefaultClient()
	}
	if fingerprint == "" {
		return base
	}
	tr, ok := base.Transport.(*http.Transport)
	if !ok || tr == nil {
		tr = http.DefaultTransport.(*http.Transport)
	}
	tr = tr.Clone()
	tr.TLSClientConfig = PinnedTLS(fingerprint)
	c := *base
	c.Transport = tr
	return &c
}

// TLSInfo is what ProbeTLS learns from a handshake.
type TLSInfo struct {
	Fingerprint string // lowercase hex SHA-256 of the leaf certificate
	// CAValid: the chain verifies against the system CAs for the host
	// (a certificate from Let's Encrypt behind a reverse proxy…).
	CAValid bool
}

// ProbeTLS connects to host:port (https URL), completes a TLS handshake
// without trusting anything and reports the certificate seen.
func ProbeTLS(ctx context.Context, rawURL string) (TLSInfo, error) {
	u, err := parseBase(rawURL)
	if err != nil {
		return TLSInfo{}, err
	}
	if u.Scheme != "https" {
		return TLSInfo{}, errors.New("agentfs: ProbeTLS needs an https URL")
	}
	host, port := u.Hostname(), u.Port()
	if port == "" {
		port = "443"
	}
	var leaf *x509.Certificate
	var inter []*x509.Certificate
	d := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 10 * time.Second},
		Config: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec // only reads the certificate; trust is decided by the caller
			ServerName:         host,
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 {
					return errors.New("agentfs: no certificate presented")
				}
				leaf, inter = cs.PeerCertificates[0], cs.PeerCertificates[1:]
				return nil
			},
		},
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return TLSInfo{}, fmt.Errorf("agentfs: TLS: %w", err)
	}
	_ = conn.Close()
	info := TLSInfo{Fingerprint: agentproto.CertFingerprint(leaf)}
	pool := x509.NewCertPool()
	for _, c := range inter {
		pool.AddCert(c)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Intermediates: pool}); err == nil {
		info.CAValid = true
	}
	return info, nil
}
