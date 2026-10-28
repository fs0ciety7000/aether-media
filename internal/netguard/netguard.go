// Package netguard builds HTTP clients that refuse to reach private,
// loopback, link-local, multicast and other non-public addresses (SSRF
// protection, T-049). It is used for every URL that does not come from a
// fixed, trusted endpoint: artwork URLs listed in NFO files, image URLs
// returned by metadata providers, redirects.
//
// The check happens when the socket is opened (net.Dialer.Control), after
// DNS resolution, on the address actually dialled: a hostname that resolves
// to a public address when validated and to 127.0.0.1 when connecting (DNS
// rebinding) is still refused. Redirects are dialled the same way.
//
// When an outbound proxy is configured (HTTP_PROXY / HTTPS_PROXY), the
// connection goes to the proxy, which the administrator chose: dialling it is
// allowed, and the target host is checked before the request is handed to it
// (literal address, or every address it resolves to). The proxy resolves the
// name again, so rebinding protection then depends on the proxy.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrBlocked is returned (wrapped) when a destination is not a public address.
var ErrBlocked = errors.New("netguard: destination address not allowed")

// blocked lists the ranges that are never public destinations. IPv4-mapped
// IPv6 addresses are unmapped before the check; NAT64 and 6to4 addresses are
// checked on their embedded IPv4 address.
var blocked = mustPrefixes(
	// IPv4
	"0.0.0.0/8",       // "this network"
	"10.0.0.0/8",      // private
	"100.64.0.0/10",   // carrier-grade NAT (also Alibaba Cloud metadata 100.100.100.200)
	"127.0.0.0/8",     // loopback
	"169.254.0.0/16",  // link-local, cloud metadata 169.254.169.254
	"172.16.0.0/12",   // private
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // documentation
	"192.88.99.0/24",  // 6to4 relay anycast
	"192.168.0.0/16",  // private
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"224.0.0.0/4",     // multicast
	"240.0.0.0/4",     // reserved, broadcast
	// IPv6
	"::/128",        // unspecified
	"::1/128",       // loopback
	"100::/64",      // discard
	"2001::/32",     // Teredo
	"2001:db8::/32", // documentation
	"fc00::/7",      // unique local (fd00::/8 included; AWS metadata fd00:ec2::254)
	"fe80::/10",     // link-local
	"fec0::/10",     // site-local (deprecated)
	"ff00::/8",      // multicast
)

var (
	nat64   = netip.MustParsePrefix("64:ff9b::/96")
	sixTo4  = netip.MustParsePrefix("2002::/16")
	nat64v2 = netip.MustParsePrefix("64:ff9b:1::/48")
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// IsPublic reports whether ip is a public unicast address.
func IsPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	if ip.Is6() {
		b := ip.As16()
		switch {
		case nat64.Contains(ip):
			return IsPublic(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
		case sixTo4.Contains(ip):
			return IsPublic(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
		case nat64v2.Contains(ip):
			return false // local-use NAT64 prefix
		}
	}
	for _, p := range blocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// Guard decides which addresses may be dialled.
type Guard struct {
	allow []netip.Prefix // exceptions (tests, an administrator's LAN artwork server)
}

// New returns a Guard that only lets public addresses through, plus the
// given exceptions.
func New(allow ...netip.Prefix) *Guard { return &Guard{allow: allow} }

// Allowed reports whether ip may be dialled.
func (g *Guard) Allowed(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, p := range g.allow {
		if p.Contains(ip) {
			return true
		}
	}
	return IsPublic(ip)
}

// Control is a net.Dialer.Control function: it runs after name resolution,
// for each address the dialer is about to connect to.
func (g *Guard) Control(network, address string, _ syscall.RawConn) error {
	switch network {
	case "tcp4", "tcp6":
	default:
		return fmt.Errorf("%w: network %s", ErrBlocked, network)
	}
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBlocked, address)
	}
	if !g.Allowed(ap.Addr()) {
		return fmt.Errorf("%w: %s", ErrBlocked, ap.Addr())
	}
	return nil
}

// checkHost validates a target host before it is handed to a proxy.
func (g *Guard) checkHost(ctx context.Context, host string) error {
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if !g.Allowed(ip) {
			return fmt.Errorf("%w: %s", ErrBlocked, ip)
		}
		return nil
	}
	if h := strings.ToLower(strings.TrimSuffix(host, ".")); h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return fmt.Errorf("%w: %s", ErrBlocked, host)
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return err
	}
	for _, a := range addrs {
		if !g.Allowed(a) {
			return fmt.Errorf("%w: %s resolves to %s", ErrBlocked, host, a.Unmap())
		}
	}
	return nil
}

// ClientOptions configures NewClient. Zero values select the defaults.
type ClientOptions struct {
	Timeout       time.Duration // whole request, body included (default 30 s)
	DialTimeout   time.Duration // default 10 s
	HeaderTimeout time.Duration // time to first response byte (default 15 s)
	MaxRedirects  int           // default 5
	Allow         []netip.Prefix
	Proxy         func(*http.Request) (*url.URL, error) // default http.ProxyFromEnvironment
	NoProxy       bool                                  // never use a proxy
}

// NewClient returns an http.Client for untrusted URLs: http and https only,
// public destinations only (checked at connect time, redirects included),
// bounded redirects and timeouts. Callers still bound the body size they read.
func NewClient(o ClientOptions) *http.Client {
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.DialTimeout <= 0 {
		o.DialTimeout = 10 * time.Second
	}
	if o.HeaderTimeout <= 0 {
		o.HeaderTimeout = 15 * time.Second
	}
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = 5
	}
	g := New(o.Allow...)
	guarded := &net.Dialer{Timeout: o.DialTimeout, KeepAlive: 30 * time.Second, Control: g.Control}
	plain := &net.Dialer{Timeout: o.DialTimeout, KeepAlive: 30 * time.Second}

	proxyFn := o.Proxy
	if proxyFn == nil {
		proxyFn = http.ProxyFromEnvironment
	}
	var proxies sync.Map // "host:port" of proxies in use → struct{}
	proxy := func(req *http.Request) (*url.URL, error) {
		if err := checkURL(req.URL); err != nil {
			return nil, err
		}
		// Literal addresses are refused up front (names are checked when dialled).
		if ip, err := netip.ParseAddr(strings.Trim(req.URL.Hostname(), "[]")); err == nil && !g.Allowed(ip) {
			return nil, fmt.Errorf("%w: %s", ErrBlocked, ip.Unmap())
		}
		if o.NoProxy {
			return nil, nil
		}
		pu, err := proxyFn(req)
		if err != nil || pu == nil {
			return nil, err
		}
		// The proxy dials the target itself: validate the target here.
		if err := g.checkHost(req.Context(), req.URL.Hostname()); err != nil {
			return nil, err
		}
		proxies.Store(proxyAddr(pu), struct{}{})
		return pu, nil
	}
	tr := &http.Transport{
		Proxy: proxy,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if _, ok := proxies.Load(addr); ok {
				return plain.DialContext(ctx, network, addr)
			}
			return guarded.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:      true,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  o.HeaderTimeout,
		ExpectContinueTimeout:  time.Second,
		IdleConnTimeout:        90 * time.Second,
		MaxIdleConns:           32,
		MaxIdleConnsPerHost:    4,
		MaxResponseHeaderBytes: 64 << 10,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   o.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= o.MaxRedirects {
				return fmt.Errorf("netguard: stopped after %d redirects", len(via))
			}
			return checkURL(req.URL)
		},
	}
}

// checkURL only lets plain http(s) URLs with a host through.
func checkURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: scheme %q", ErrBlocked, u.Scheme)
	}
	return nil
}

func proxyAddr(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}
