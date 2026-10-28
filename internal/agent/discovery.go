package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"
)

// DiscoveryMagic is the UDP request an Aether server broadcasts to find
// agents on the LAN.
const DiscoveryMagic = "AETHER_DISCOVER_AGENT_V1"

const (
	discoveryMaxPacket   = 64 // anything longer is not a discovery request
	discoveryMaxReply    = 2048
	discoveryRepliesPerS = 50 // global cap: limits reflection abuse
)

// discoveryReply is the JSON answer sent back (unicast) to the requester.
type discoveryReply struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	Port     int    `json:"port"`
	TLS      bool   `json:"tls"`
}

// Discovered is an agent found by Discover.
type Discovered struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	Port     int    `json:"port"`
	TLS      bool   `json:"tls"`
	Addr     string `json:"addr"` // source IP of the reply
	URL      string `json:"url"`  // http(s)://Addr:Port
}

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// localSource reports whether ip may receive a discovery reply: loopback,
// private, link-local or CGNAT (Tailscale) addresses only, so the agent
// cannot be used to reflect traffic towards the Internet.
func localSource(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip)
}

// discoveryAddr is the UDP address of the responder: loopback when the HTTP
// listener is loopback-only, every interface otherwise (broadcasts are only
// delivered to sockets bound to the wildcard address).
func (s *Server) discoveryAddr() string {
	host, _, _ := net.SplitHostPort(s.cfg.Listen)
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return net.JoinHostPort(host, strconv.Itoa(s.cfg.DiscoveryPort))
	}
	return ":" + strconv.Itoa(s.cfg.DiscoveryPort)
}

// serveDiscovery answers discovery requests on conn until ctx is done.
func (s *Server) serveDiscovery(ctx context.Context, conn net.PacketConn) {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	buf := make([]byte, 512)
	var winStart time.Time
	replies := 0
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			s.log.Debug("découverte : lecture", "err", err)
			continue
		}
		if n > discoveryMaxPacket || !bytes.Equal(bytes.TrimSpace(buf[:n]), []byte(DiscoveryMagic)) {
			continue
		}
		ua, ok := from.(*net.UDPAddr)
		if !ok || ua.Port == 0 || !localSource(ua.IP) {
			continue
		}
		if now := time.Now(); now.Sub(winStart) >= time.Second {
			winStart, replies = now, 0
		}
		if replies >= discoveryRepliesPerS {
			continue
		}
		replies++
		b, err := json.Marshal(discoveryReply{
			UID: s.uid, Name: s.name, Hostname: s.hostname, Version: s.version,
			Port: int(s.httpPort.Load()), TLS: s.cfg.TLSCert != "",
		})
		if err != nil {
			continue
		}
		_, _ = conn.WriteTo(b, from)
	}
}

// Discover broadcasts a discovery request on the LAN (255.255.255.255 and
// each interface's broadcast address) to the given UDP port and collects
// the agents' replies until timeout, deduplicated by uid.
func Discover(ctx context.Context, port int, timeout time.Duration) ([]Discovered, error) {
	return discover(ctx, port, timeout, broadcastTargets())
}

func broadcastTargets() []net.IP {
	out := []net.IP{net.IPv4bcast}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagBroadcast == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipn.IP.To4()
			if v4 == nil || len(ipn.Mask) != net.IPv4len {
				continue
			}
			bc := make(net.IP, net.IPv4len)
			for i := range bc {
				bc[i] = v4[i] | ^ipn.Mask[i]
			}
			out = append(out, bc)
		}
	}
	return out
}

// discover sends the request to every target (twice, against packet loss)
// and gathers replies. Tests target 127.0.0.1 directly.
func discover(ctx context.Context, port int, timeout time.Duration, targets []net.IP) ([]Discovered, error) {
	if port <= 0 || port > 65535 {
		return nil, errors.New("agent: port de découverte invalide")
	}
	conn, err := net.ListenUDP("udp4", nil) // Go enables SO_BROADCAST on UDP sockets
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	send := func() int {
		ok := 0
		for _, ip := range targets {
			if _, err := conn.WriteToUDP([]byte(DiscoveryMagic), &net.UDPAddr{IP: ip, Port: port}); err == nil {
				ok++
			}
		}
		return ok
	}
	if send() == 0 {
		return nil, errors.New("agent: impossible d'envoyer la requête de découverte")
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		resend := min(timeout/3, 500*time.Millisecond)
		t := time.NewTimer(resend)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
			send()
		}
	})
	defer wg.Wait()

	var out []Discovered
	seen := map[string]bool{}
	buf := make([]byte, discoveryMaxReply)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break // closed at the deadline or on cancellation
		}
		var rep discoveryReply
		if json.Unmarshal(buf[:n], &rep) != nil || rep.UID == "" || len(rep.UID) > 64 ||
			rep.Port <= 0 || rep.Port > 65535 || seen[rep.UID] {
			continue
		}
		seen[rep.UID] = true
		addr := from.IP.String()
		if v4 := from.IP.To4(); v4 != nil {
			addr = v4.String()
		}
		scheme := "http"
		if rep.TLS {
			scheme = "https"
		}
		out = append(out, Discovered{
			UID: rep.UID, Name: rep.Name, Hostname: rep.Hostname, Version: rep.Version,
			Port: rep.Port, TLS: rep.TLS, Addr: addr,
			URL: scheme + "://" + net.JoinHostPort(addr, strconv.Itoa(rep.Port)),
		})
	}
	if err := context.Cause(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return out, err
	}
	return out, nil
}
