package netguard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func TestIsPublic(t *testing.T) {
	for _, s := range []string{
		"0.0.0.0", "10.1.2.3", "100.100.100.200", "127.0.0.1", "127.8.9.10", "169.254.169.254",
		"172.16.0.1", "172.31.255.255", "192.168.1.10", "192.0.2.1", "198.18.0.1", "224.0.0.251",
		"239.255.255.250", "255.255.255.255", "240.0.0.1",
		"::", "::1", "fe80::1", "fc00::1", "fd00::1", "fd00:ec2::254", "ff02::1", "fec0::1", "2001:db8::1",
		"::ffff:127.0.0.1", "::ffff:169.254.169.254", "64:ff9b::7f00:1", "64:ff9b::a9fe:a9fe",
		"2002:7f00:1::1", "2002:c0a8:101::1", "64:ff9b:1::1", "2001::1",
	} {
		if IsPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s: public, want blocked", s)
		}
	}
	for _, s := range []string{
		"8.8.8.8", "1.1.1.1", "151.101.1.1", "52.84.150.10", "172.32.0.1", "100.128.0.1", "2606:4700::1111",
		"2a00:1450:4007:80f::200e", "64:ff9b::808:808", "2002:808:808::1", "::ffff:8.8.8.8",
	} {
		if !IsPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s: blocked, want public", s)
		}
	}
	if IsPublic(netip.MustParseAddr("fe80::1%eth0")) || IsPublic(netip.Addr{}) {
		t.Error("zoned or invalid address must be blocked")
	}
}

func TestControl(t *testing.T) {
	g := New(netip.MustParsePrefix("192.168.1.0/24"))
	for addr, ok := range map[string]bool{
		"8.8.8.8:443":        true,
		"[2606:4700::1]:80":  true,
		"192.168.1.20:80":    true, // explicit exception
		"192.168.2.20:80":    false,
		"127.0.0.1:8096":     false,
		"[::1]:8096":         false,
		"169.254.169.254:80": false,
		"[fd00:ec2::254]:80": false,
		"not-an-ip:80":       false,
	} {
		err := g.Control("tcp4", addr, nil)
		if (err == nil) != ok {
			t.Errorf("%s: err = %v, want allowed=%v", addr, err, ok)
		}
		if err != nil && !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: %v is not ErrBlocked", addr, err)
		}
	}
	if err := g.Control("udp4", "8.8.8.8:53", nil); !errors.Is(err, ErrBlocked) {
		t.Errorf("udp: %v", err)
	}
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "secret") }))
	t.Cleanup(srv.Close)
	return srv
}

func get(c *http.Client, u string) error {
	resp, err := c.Get(u)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func TestClientBlocksPrivateDestinations(t *testing.T) {
	srv := okServer(t)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	c := NewClient(ClientOptions{NoProxy: true})
	for _, u := range []string{
		srv.URL, // 127.0.0.1
		fmt.Sprintf("http://localhost:%d/", port), // name resolved to loopback, refused at connect time
		fmt.Sprintf("http://[::ffff:127.0.0.1]:%d/", port),
		"http://169.254.169.254/latest/meta-data/",
		"http://[fd00:ec2::254]/latest/meta-data/",
		"http://0.0.0.0:1/",
	} {
		if err := get(c, u); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: err = %v, want ErrBlocked", u, err)
		}
	}
	for _, u := range []string{"ftp://example.com/a.png", "file:///etc/passwd", "gopher://127.0.0.1:70/"} {
		if err := get(c, u); err == nil {
			t.Errorf("%s: expected an error", u)
		}
	}
	// An explicit exception lets the same server through.
	allowed := NewClient(ClientOptions{NoProxy: true, Allow: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}})
	if err := get(allowed, srv.URL); err != nil {
		t.Fatalf("allowed: %v", err)
	}
}

func TestClientChecksRedirects(t *testing.T) {
	// The target lives on 127.0.0.2, outside the exception granted to the
	// first hop (127.0.0.1): following the redirect must be refused at connect.
	ln, err := net.Listen("tcp4", "127.0.0.2:0")
	if err != nil {
		t.Skipf("127.0.0.2 unavailable: %v", err)
	}
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "internal") }))
	target.Listener = ln
	target.Start()
	defer target.Close()

	hops := 0
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-internal":
			http.Redirect(w, r, target.URL+"/", http.StatusFound)
		case "/to-file":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/loop":
			hops++
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			_, _ = io.WriteString(w, "ok")
		}
	}))
	defer front.Close()

	c := NewClient(ClientOptions{NoProxy: true, Allow: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}})
	if err := get(c, front.URL+"/"); err != nil {
		t.Fatalf("front: %v", err)
	}
	if err := get(c, front.URL+"/to-internal"); !errors.Is(err, ErrBlocked) {
		t.Errorf("redirect to 127.0.0.2: %v, want ErrBlocked", err)
	}
	if err := get(c, front.URL+"/to-file"); err == nil {
		t.Error("redirect to file://: expected an error")
	}
	if err := get(c, front.URL+"/loop"); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("redirect loop: %v", err)
	}
	if hops != 5 {
		t.Errorf("hops = %d, want 5", hops)
	}
}

// A forward proxy on loopback (as in many corporate or sandboxed setups)
// is dialled, but the target is still checked before it is handed over.
func TestClientThroughProxy(t *testing.T) {
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.String())
		_, _ = io.WriteString(w, "via proxy")
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)
	c := NewClient(ClientOptions{Proxy: http.ProxyURL(pu)})

	if err := get(c, "http://8.8.8.8/poster.jpg"); err != nil {
		t.Fatalf("public target via proxy: %v", err)
	}
	for _, u := range []string{"http://10.0.0.1/", "http://169.254.169.254/", "http://localhost/", "http://[::1]/"} {
		if err := get(c, u); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s via proxy: %v, want ErrBlocked", u, err)
		}
	}
	if len(seen) != 1 || seen[0] != "http://8.8.8.8/poster.jpg" {
		t.Errorf("proxy saw %v", seen)
	}
}

func TestCheckHostResolves(t *testing.T) {
	g := New()
	if err := g.checkHost(context.Background(), "localhost"); !errors.Is(err, ErrBlocked) {
		t.Errorf("localhost: %v", err)
	}
	if err := g.checkHost(context.Background(), "[::1]"); !errors.Is(err, ErrBlocked) {
		t.Errorf("[::1]: %v", err)
	}
	if err := g.checkHost(context.Background(), "8.8.8.8"); err != nil {
		t.Errorf("8.8.8.8: %v", err)
	}
}
