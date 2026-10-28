package sources

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

func TestLocations(t *testing.T) {
	remote := db.LibrarySource{ID: 7, Kind: KindSFTP, Root: "/data/media"}
	loc := Location(remote, "Films/Heat (1995)/heat.mkv")
	if loc != "src:7:/Films/Heat (1995)/heat.mkv" {
		t.Fatalf("loc = %q", loc)
	}
	// Remote locations behave like paths for the resolvers.
	if d := filepath.Dir(loc); d != "src:7:/Films/Heat (1995)" {
		t.Errorf("Dir = %q", d)
	}
	if j := filepath.Join(RootLocation(remote), "Films"); j != "src:7:/Films" {
		t.Errorf("Join = %q", j)
	}
	id, rel, ok := ParseRemote(loc)
	if !ok || id != 7 || rel != "Films/Heat (1995)/heat.mkv" {
		t.Errorf("ParseRemote = %d %q %v", id, rel, ok)
	}
	if _, _, ok := ParseRemote("/srv/media/heat.mkv"); ok {
		t.Error("local path parsed as remote")
	}
	if _, rel, _ := ParseRemote("src:7:/../../etc/passwd"); strings.Contains(rel, "..") {
		t.Errorf("traversal kept: %q", rel)
	}
	local := db.LibrarySource{ID: 1, Kind: KindLocal, Root: "/srv/media"}
	if l := Location(local, "a/b.mkv"); l != filepath.Join("/srv/media", "a", "b.mkv") {
		t.Errorf("local loc = %q", l)
	}
}

func TestProxySignature(t *testing.T) {
	m := &Manager{proxyKey: []byte("0123456789abcdef0123456789abcdef"), ProxyBase: "http://127.0.0.1:8096"}
	u := m.proxyURL(3, "Films/a b.mkv", time.Hour)
	parts := strings.Split(strings.TrimPrefix(u, "http://127.0.0.1:8096/internal/vfs/"), "/")
	if len(parts) < 4 || parts[0] != "3" || !strings.Contains(u, "a%20b.mkv") {
		t.Fatalf("url = %s", u)
	}
	var exp int64
	for _, c := range parts[1] {
		exp = exp*10 + int64(c-'0')
	}
	if loc, ok := m.VerifyProxy(3, exp, parts[2], "Films/a b.mkv"); !ok || loc != "src:3:/Films/a b.mkv" {
		t.Errorf("verify = %q %v", loc, ok)
	}
	if _, ok := m.VerifyProxy(3, exp, parts[2], "Films/other.mkv"); ok {
		t.Error("signature must bind the path")
	}
	if _, ok := m.VerifyProxy(4, exp, parts[2], "Films/a b.mkv"); ok {
		t.Error("signature must bind the source")
	}
	if _, ok := m.VerifyProxy(3, time.Now().Add(-time.Minute).Unix(), parts[2], "Films/a b.mkv"); ok {
		t.Error("expired signature accepted")
	}
}

func TestPrepareValidation(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		in Input
		ok bool
	}{
		{Input{Kind: KindLocal, Root: dir}, true},
		{Input{Kind: KindLocal, Root: "relative"}, false},
		{Input{Kind: KindAgent, Agent: &AgentIn{URL: "http://nas:8097", Token: "t", Share: "films"}}, true},
		{Input{Kind: KindAgent, Agent: &AgentIn{URL: "ftp://nas", Token: "t", Share: "films"}}, false},
		{Input{Kind: KindAgent, Root: "../etc", Agent: &AgentIn{URL: "http://nas:8097", Token: "t", Share: "films"}}, false},
		{Input{Kind: KindAgent, Agent: &AgentIn{URL: "http://nas:8097", Share: "films"}}, false}, // no token
		{Input{Kind: KindSFTP, Root: "/home/u/media", SFTP: &SFTPInput{Host: "box", User: "u", Password: "p"}}, true},
		{Input{Kind: KindSFTP, Root: "media", SFTP: &SFTPInput{Host: "box", User: "u", Password: "p"}}, false},
		{Input{Kind: KindSFTP, Root: "/m", SFTP: &SFTPInput{Host: "box", User: "u"}}, false}, // no credentials
		{Input{Kind: "ftp"}, false},
	}
	for i, c := range cases {
		p, err := prepare(c.in, nil)
		var ve *ValidationError
		if c.ok && err != nil || !c.ok && !errors.As(err, &ve) {
			t.Errorf("case %d: %+v → %v", i, c.in, err)
		}
		if c.ok && p.name == "" {
			t.Errorf("case %d: default name missing", i)
		}
	}
	// Updates keep stored secrets when the client sends none.
	prev := Secrets{Token: "old"}
	p, err := prepare(Input{Kind: KindAgent, Agent: &AgentIn{URL: "http://nas:8097", Share: "films"}}, &prev)
	if err != nil || p.sec.Token != "old" {
		t.Errorf("update kept token? %+v %v", p.sec, err)
	}
}
