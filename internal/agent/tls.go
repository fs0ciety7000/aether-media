package agent

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agentproto"
)

const (
	certFileName = "agent-tls.crt"
	keyFileName  = "agent-tls.key"
	// certValidity is long on purpose: the server pins the certificate by
	// fingerprint, so renewing it means pairing again.
	certValidity = 20 * 365 * 24 * time.Hour
)

// Certificate origins reported to the server when pairing.
const (
	certSelfSigned = "self-signed"
	certCustom     = "custom"
)

// agentTLS is the certificate the agent serves.
type agentTLS struct {
	cert        tls.Certificate
	fingerprint string // lowercase hex SHA-256 of the leaf
	source      string // certSelfSigned | certCustom
}

func (t *agentTLS) config() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{t.cert},
		MinVersion:   tls.VersionTLS12,
		// HTTP/2 lets the server multiplex all its calls on one connection
		// (http.Server.Serve configures it for the *tls.Conn the sniffing
		// listener returns, since Server.TLSConfig is nil).
		NextProtos: []string{"h2", "http/1.1"},
	}
}

// loadTLS loads tls_cert/tls_key when configured, else the self-signed
// certificate of dataDir (created on first start, files 0600). created
// reports a new certificate.
func loadTLS(cfg Config, hostname string) (t *agentTLS, created bool, err error) {
	if cfg.TLSCert != "" {
		c, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, false, fmt.Errorf("agent: certificat TLS : %w", err)
		}
		return &agentTLS{cert: c, fingerprint: agentproto.Fingerprint(c.Certificate[0]), source: certCustom}, false, nil
	}
	cp := filepath.Join(cfg.DataDir, certFileName)
	kp := filepath.Join(cfg.DataDir, keyFileName)
	c, err := tls.LoadX509KeyPair(cp, kp)
	switch {
	case err == nil:
		for _, p := range []string{cp, kp} {
			if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 {
				if err := os.Chmod(p, 0o600); err != nil {
					return nil, false, fmt.Errorf("agent: permissions de %s : %w", p, err)
				}
			}
		}
		return &agentTLS{cert: c, fingerprint: agentproto.Fingerprint(c.Certificate[0]), source: certSelfSigned}, false, nil
	case !errors.Is(err, os.ErrNotExist):
		return nil, false, fmt.Errorf("agent: certificat %s illisible (%w) ; supprimez %s et %s pour en générer un nouveau (il faudra réappairer)", cp, err, certFileName, keyFileName)
	}
	if _, kerr := os.Stat(kp); kerr == nil {
		return nil, false, fmt.Errorf("agent: %s existe sans %s ; supprimez-le pour régénérer le certificat", kp, cp)
	}
	certPEM, keyPEM, err := selfSigned(hostname, time.Now())
	if err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, false, fmt.Errorf("agent: création de %s : %w", cfg.DataDir, err)
	}
	// Key first: a crash in between leaves a key without certificate,
	// reported above rather than silently replaced.
	if err := writeExclusive(kp, keyPEM); err != nil {
		return nil, false, err
	}
	if err := writeExclusive(cp, certPEM); err != nil {
		_ = os.Remove(kp)
		return nil, false, err
	}
	c, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, false, err
	}
	return &agentTLS{cert: c, fingerprint: agentproto.Fingerprint(c.Certificate[0]), source: certSelfSigned}, true, nil
}

func writeExclusive(p string, b []byte) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("agent: écriture de %s : %w", p, err)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(p)
		return fmt.Errorf("agent: écriture de %s : %w", p, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("agent: écriture de %s : %w", p, err)
	}
	return f.Close()
}

// selfSigned creates an ECDSA P-256 certificate valid for the host name,
// localhost and the machine's current IP addresses. The SANs only matter to
// browsers opening the local page: the server pins the fingerprint.
func selfSigned(hostname string, now time.Time) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, err
	}
	cn := "Aether agent"
	if hostname != "" {
		cn += " " + hostname
	}
	dns := []string{"localhost"}
	if hostname != "" && hostname != "localhost" {
		dns = append([]string{hostname}, dns...)
	}
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() {
				ips = append(ips, ipn.IP)
			}
		}
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn, Organization: []string{"Aether"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(certValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dns,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder}), nil
}

// ---- one port, HTTPS and (compatibility) HTTP ----

// sniffListener serves TLS and plain HTTP on the same port: the first byte
// of a TLS connection is a handshake record (0x16), never the first byte of
// an HTTP request. Plain HTTP is then restricted by the handlers (local
// page, servers paired over HTTP before the switch to HTTPS).
type sniffListener struct {
	net.Listener
	cfg   *tls.Config
	conns chan net.Conn
	errs  chan error
	done  chan struct{}
	once  sync.Once
}

const sniffTimeout = 10 * time.Second

func newSniffListener(ln net.Listener, cfg *tls.Config) *sniffListener {
	l := &sniffListener{Listener: ln, cfg: cfg, conns: make(chan net.Conn), errs: make(chan error), done: make(chan struct{})}
	go l.acceptLoop()
	return l
}

func (l *sniffListener) acceptLoop() {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			select {
			case l.errs <- err:
			case <-l.done:
				return
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go l.classify(c)
	}
}

func (l *sniffListener) classify(c net.Conn) {
	_ = c.SetReadDeadline(time.Now().Add(sniffTimeout))
	br := bufio.NewReaderSize(c, 512)
	first, err := br.Peek(1)
	if err != nil {
		_ = c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	pc := &peekedConn{Conn: c, r: br}
	var out net.Conn = pc
	if first[0] == 0x16 {
		out = tls.Server(pc, l.cfg)
	}
	select {
	case l.conns <- out:
	case <-l.done:
		_ = c.Close()
	}
}

func (l *sniffListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case err := <-l.errs:
		return nil, err
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *sniffListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

// peekedConn replays the bytes buffered while sniffing.
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
