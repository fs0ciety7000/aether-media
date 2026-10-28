package agentfs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
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

var (
	clients sync.Map // clientKey → *http.Client
	pinned  sync.Map // pinKey → *http.Client (PinClient of a caller's client)
)

type pinKey struct {
	base        *http.Client
	fingerprint string
}

// clientKey identifies one agent endpoint: one transport, hence one
// connection pool (and one HTTP/2 connection), per agent and certificate.
func clientKey(baseURL, fingerprint string) string {
	if u, err := parseBase(baseURL); err == nil {
		baseURL = strings.ToLower(u.Scheme + "://" + u.Host)
	}
	return baseURL + "|" + fingerprint
}

// ClientFor returns the HTTP client shared by every call to the agent at
// baseURL (API, probe, walk, change stream, ranged reads): one transport,
// so connections are reused (keep-alive, HTTP/2) and bounded
// (MaxConnsPerAgent). fingerprint pins the certificate ("" verifies it
// against the system CAs, as for any HTTPS site, and is also used for plain
// HTTP agents). No overall timeout: bodies are long media streams; callers
// bound metadata calls with contexts.
func ClientFor(baseURL, fingerprint string) *http.Client {
	k := clientKey(baseURL, fingerprint)
	if c, ok := clients.Load(k); ok {
		return c.(*http.Client)
	}
	c, _ := clients.LoadOrStore(k, newPinned(nil, fingerprint))
	return c.(*http.Client)
}

// PinClient returns base (DefaultClient when nil) with a transport pinning
// fingerprint. An empty fingerprint returns base unchanged. The pinned copy
// of a given base client is built once and shared, so that repeated calls
// do not each start a new connection pool.
func PinClient(base *http.Client, fingerprint string) *http.Client {
	if base == nil {
		return newPinned(nil, fingerprint)
	}
	if fingerprint == "" {
		return base
	}
	k := pinKey{base, fingerprint}
	if c, ok := pinned.Load(k); ok {
		return c.(*http.Client)
	}
	c, _ := pinned.LoadOrStore(k, newPinned(base, fingerprint))
	return c.(*http.Client)
}

func newPinned(base *http.Client, fingerprint string) *http.Client {
	if base == nil {
		base = DefaultClient()
	}
	if fingerprint == "" {
		return base
	}
	var tr *http.Transport
	switch bt := base.Transport.(type) {
	case *agentTransport:
		tr = bt.Clone() // the inner *http.Transport
	case *http.Transport:
		tr = bt.Clone()
	default:
		tr = newTransport()
	}
	tr.TLSClientConfig = PinnedTLS(fingerprint)
	tr.ForceAttemptHTTP2 = true
	c := *base
	c.Transport = wrapTransport(tr)
	return &c
}

// agentTransport avoids a burst of handshakes when several requests start
// together on a cold transport: net/http dials one connection per waiting
// request before it knows that the first one will speak HTTP/2 (it then
// closes the extra ones, but the TCP and TLS handshakes happened). After an
// idle period the first request therefore goes alone until it has its
// connection; the others then share it (HTTP/2) or reuse/open keep-alive
// connections as usual (HTTP/1.1).
type agentTransport struct {
	*http.Transport
	first    chan struct{} // held by the request going alone
	lastUse  atomic.Int64  // unix nano of the last connection obtained
	coldIdle time.Duration
}

func wrapTransport(tr *http.Transport) *agentTransport {
	idle := tr.IdleConnTimeout
	if idle <= 0 {
		idle = 90 * time.Second
	}
	return &agentTransport{Transport: tr, first: make(chan struct{}, 1), coldIdle: idle * 2 / 3}
}

func (t *agentTransport) warm() bool {
	return time.Since(time.Unix(0, t.lastUse.Load())) < t.coldIdle
}

func (t *agentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.warm() {
		return t.trace(req, nil)
	}
	select {
	case t.first <- struct{}{}:
	case <-req.Context().Done():
		if req.Body != nil {
			_ = req.Body.Close() // RoundTrip always closes the body
		}
		return nil, req.Context().Err()
	}
	var once sync.Once
	unlock := func() { once.Do(func() { <-t.first }) }
	if t.warm() {
		unlock()
		return t.trace(req, nil)
	}
	defer unlock() // dial failure: the next request tries alone
	return t.trace(req, unlock)
}

// trace runs req, recording when it got a connection.
func (t *agentTransport) trace(req *http.Request, gotConn func()) (*http.Response, error) {
	ct := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) {
		t.lastUse.Store(time.Now().UnixNano())
		if gotConn != nil {
			gotConn()
		}
	}}
	return t.Transport.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), ct)))
}

// CloseIdleConnections also forgets that the transport was warm.
func (t *agentTransport) CloseIdleConnections() {
	t.lastUse.Store(0)
	t.Transport.CloseIdleConnections()
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
