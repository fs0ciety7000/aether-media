package sftpfs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"io/fs"
	mrand "math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// testServer is an in-process SSH server exposing the sftp subsystem over
// the real file system.
type testServer struct {
	ln          net.Listener
	fingerprint string
	clientKey   []byte // PEM private key accepted by the server
	port        int

	mu    sync.Mutex
	conns []net.Conn
	wg    sync.WaitGroup
}

const testPassword = "s3cret"

func newServer(t testing.TB) *testServer {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	clientPub, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(clientPriv, "")
	if err != nil {
		t.Fatal(err)
	}
	sshClientPub, _ := ssh.NewPublicKey(clientPub)

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if c.User() == "alice" && string(pw) == testPassword {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == "alice" && bytes.Equal(k.Marshal(), sshClientPub.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
	}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &testServer{
		ln:          ln,
		fingerprint: ssh.FingerprintSHA256(hostSigner.PublicKey()),
		clientKey:   pem.EncodeToMemory(block),
		port:        ln.Addr().(*net.TCPAddr).Port,
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, nc)
			s.mu.Unlock()
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handle(nc, cfg)
			}()
		}
	}()
	t.Cleanup(s.close)
	return s
}

func (s *testServer) handle(nc net.Conn, cfg *ssh.ServerConfig) {
	defer nc.Close()
	sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			_ = nch.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, err := nch.Accept()
		if err != nil {
			return
		}
		go func() {
			for req := range creqs {
				ok := req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp"
				_ = req.Reply(ok, nil)
				if ok {
					go func() {
						srv, err := sftp.NewServer(ch)
						if err != nil {
							return
						}
						_ = srv.Serve()
						_ = srv.Close()
					}()
				}
			}
		}()
	}
}

// dropAll closes every live connection (simulates a network failure).
func (s *testServer) dropAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.conns = nil
}

func (s *testServer) close() {
	_ = s.ln.Close()
	s.dropAll()
	s.wg.Wait()
}

func (s *testServer) cfg(root string) Config {
	return Config{Host: "127.0.0.1", Port: s.port, User: "alice", Password: testPassword,
		HostKey: s.fingerprint, Root: root, Timeout: 5 * time.Second}
}

func mkTree(t testing.TB) (base, root string) {
	t.Helper()
	base = t.TempDir()
	root = filepath.Join(base, "root")
	files := map[string]string{
		"Films/Heat (1995)/heat.mkv": "heat-video",
		"Films/Heat (1995)/heat.nfo": "<movie/>",
		"Series/S01/e1.mkv":          "e1",
		"a.txt":                      "a",
	}
	for p, c := range files {
		fp := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fp, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "secret.txt"), filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "inside.txt")); err != nil {
		t.Fatal(err)
	}
	return base, root
}

func TestHostKeyTOFU(t *testing.T) {
	s := newServer(t)
	_, root := mkTree(t)
	ctx := context.Background()

	fp, err := Probe(ctx, Config{Host: "127.0.0.1", Port: s.port})
	if err != nil || fp != s.fingerprint {
		t.Fatalf("Probe = %q, %v; want %q", fp, err, s.fingerprint)
	}

	cfg := s.cfg(root)
	cfg.HostKey = ""
	_, err = New(ctx, cfg)
	var hk *HostKeyError
	if !errors.As(err, &hk) || hk.Mismatch || hk.Fingerprint != s.fingerprint {
		t.Fatalf("empty host key: %v", err)
	}

	cfg.HostKey = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, err = New(ctx, cfg)
	if !errors.As(err, &hk) || !hk.Mismatch || hk.Fingerprint != s.fingerprint {
		t.Fatalf("wrong host key: %v", err)
	}

	cfg.HostKey = s.fingerprint[len("SHA256:"):] + "=" // tolerant format
	f, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAuth(t *testing.T) {
	s := newServer(t)
	_, root := mkTree(t)
	ctx := context.Background()

	cfg := s.cfg(root)
	cfg.Password = ""
	cfg.PrivateKey = s.clientKey
	f, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("key auth: %v", err)
	}
	if err := f.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := f.Ping(ctx); !errors.Is(err, ErrClosed) {
		t.Fatalf("ping after close: %v", err)
	}

	cfg = s.cfg(root)
	cfg.Password = "wrong"
	if _, err := New(ctx, cfg); !errors.Is(err, ErrAuth) {
		t.Fatalf("bad password: %v", err)
	}
	cfg.PrivateKey = []byte("garbage")
	if _, err := New(ctx, cfg); err == nil {
		t.Fatal("garbage key accepted")
	}
	cfg = s.cfg("relative/path")
	if _, err := New(ctx, cfg); !errors.Is(err, vfs.ErrPath) {
		t.Fatalf("relative root: %v", err)
	}
}

func newFS(t testing.TB, s *testServer, root string) vfs.FS {
	t.Helper()
	f, err := New(context.Background(), s.cfg(root))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestWalkStatReadDir(t *testing.T) {
	s := newServer(t)
	_, root := mkTree(t)
	f := newFS(t, s, root)
	ctx := context.Background()

	var got []string
	if err := f.Walk(ctx, "", func(e vfs.Entry) error {
		got = append(got, e.Path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Films": true, "Films/Heat (1995)": true, "Films/Heat (1995)/heat.mkv": true,
		"Films/Heat (1995)/heat.nfo": true, "Series": true, "Series/S01": true, "Series/S01/e1.mkv": true,
		"a.txt": true, "inside.txt": true}
	if len(got) != len(want) {
		t.Fatalf("walk = %v", got)
	}
	seen := map[string]bool{}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected %q in %v", p, got)
		}
		if d := filepath.Dir(p); d != "." && !seen[d] {
			t.Fatalf("%q before its parent", p)
		}
		seen[p] = true
	}

	got = nil
	if err := f.Walk(ctx, "Films", func(e vfs.Entry) error {
		got = append(got, e.Path)
		if e.IsDir {
			return fs.SkipDir
		}
		return nil
	}); err != nil || len(got) != 1 {
		t.Fatalf("skipdir: %v %v", got, err)
	}

	e, err := f.Stat(ctx, "Films/Heat (1995)/heat.mkv")
	if err != nil || e.Size != 10 || e.IsDir || e.Path != "Films/Heat (1995)/heat.mkv" {
		t.Fatalf("stat = %+v %v", e, err)
	}
	if _, err := f.Stat(ctx, "nope"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("stat missing: %v", err)
	}
	if err := f.Walk(ctx, "nope", func(vfs.Entry) error { return nil }); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("walk missing: %v", err)
	}
	es, err := f.ReadDir(ctx, "")
	if err != nil || len(es) != 4 || es[0].Path != "Films" || es[3].Path != "inside.txt" || es[3].Size != 1 {
		t.Fatalf("readdir = %+v %v", es, err)
	}
	if f.InputURL("a.txt") != "" {
		t.Fatal("InputURL should be empty")
	}
}

func TestConfinement(t *testing.T) {
	s := newServer(t)
	_, root := mkTree(t)
	f := newFS(t, s, root)
	ctx := context.Background()
	for _, p := range []string{"../secret.txt", "/etc/passwd", "Films/../../secret.txt", "a/../../x"} {
		if _, err := f.Open(ctx, p); !errors.Is(err, vfs.ErrPath) {
			t.Errorf("Open(%q) = %v; want ErrPath", p, err)
		}
		if _, err := f.Stat(ctx, p); !errors.Is(err, vfs.ErrPath) {
			t.Errorf("Stat(%q) = %v; want ErrPath", p, err)
		}
	}
	// Symlink to outside: refused and not listed.
	if _, err := f.Open(ctx, "escape.txt"); !errors.Is(err, vfs.ErrPath) {
		t.Errorf("Open(escape.txt) = %v; want ErrPath", err)
	}
	if fl, err := f.Open(ctx, "inside.txt"); err != nil {
		t.Errorf("Open(inside.txt) = %v", err)
	} else {
		fl.Close()
	}
	es, _ := f.ReadDir(ctx, "")
	for _, e := range es {
		if e.Path == "escape.txt" {
			t.Fatal("escaping symlink listed")
		}
	}
	if w := within("/srv/media", "/srv/media2/x"); w {
		t.Fatal("prefix confusion")
	}
}

func TestOpenSeekRead(t *testing.T) {
	s := newServer(t)
	_, root := mkTree(t)
	data := make([]byte, 5<<20+123)
	for i := range data {
		data[i] = byte(mrand.IntN(256))
	}
	if err := os.WriteFile(filepath.Join(root, "big.bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFS(t, s, root)
	fl, err := f.Open(context.Background(), "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer fl.Close()
	if fl.Size() != int64(len(data)) || fl.ModTime().IsZero() {
		t.Fatal("bad size/mtime")
	}
	got, err := io.ReadAll(fl)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("readall: %v len=%d", err, len(got))
	}
	for range 200 {
		off, _ := fl.Seek(mrand.Int64N(int64(len(data))+10), io.SeekStart)
		if mrand.IntN(4) == 0 {
			off, _ = fl.Seek(-mrand.Int64N(1000), io.SeekCurrent)
		}
		buf := make([]byte, mrand.IntN(300000))
		n, _ := io.ReadFull(fl, buf)
		start := min(off, int64(len(data)))
		end := max(min(off+int64(len(buf)), int64(len(data))), start)
		if int64(n) != end-start || !bytes.Equal(buf[:n], data[start:end]) {
			t.Fatalf("mismatch at %d (n=%d)", off, n)
		}
	}
	if _, err := f.Open(context.Background(), "Films"); !errors.Is(err, vfs.ErrPath) {
		t.Fatalf("open dir: %v", err)
	}
	if _, err := f.Open(context.Background(), "missing"); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("open missing: %v", err)
	}
}

func TestReconnect(t *testing.T) {
	s := newServer(t)
	_, root := mkTree(t)
	data := bytes.Repeat([]byte("abcdefgh"), 1<<17) // 1 MiB
	if err := os.WriteFile(filepath.Join(root, "m.bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFS(t, s, root)
	ctx := context.Background()
	fl, err := f.Open(ctx, "m.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer fl.Close()
	head := make([]byte, 1000)
	if _, err := io.ReadFull(fl, head); err != nil {
		t.Fatal(err)
	}

	s.dropAll()
	// Wait for the client to notice (watcher) — or the next call notices.
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := f.Ping(ctx)
		if err == nil {
			break
		}
		if !errors.Is(err, vfs.ErrOffline) {
			t.Fatalf("ping after drop: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("did not reconnect: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := f.Stat(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	// The open file resumes on the new connection.
	if _, err := fl.Seek(500000, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 100)
	if _, err := io.ReadFull(fl, b); err != nil || !bytes.Equal(b, data[500000:500100]) {
		t.Fatalf("read after reconnect: %v", err)
	}

	// Server fully down → ErrOffline; then back.
	s.close()
	if err := f.Ping(ctx); !errors.Is(err, vfs.ErrOffline) {
		// The first call may hit the dying connection; the next must fail to dial.
		if err := f.Ping(ctx); !errors.Is(err, vfs.ErrOffline) {
			t.Fatalf("ping server down: %v", err)
		}
	}
}

func TestOfflineAtStart(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	f, err := New(context.Background(), Config{Host: "127.0.0.1", Port: port, User: "u", Password: "p",
		HostKey: "SHA256:x", Root: "/", Timeout: time.Second})
	if err != nil {
		t.Fatalf("offline at start should not fail New: %v", err)
	}
	defer f.Close()
	if err := f.Ping(context.Background()); !errors.Is(err, vfs.ErrOffline) {
		t.Fatalf("ping: %v", err)
	}
	if _, err := Probe(context.Background(), Config{Host: "127.0.0.1", Port: port}); !errors.Is(err, vfs.ErrOffline) {
		t.Fatalf("probe: %v", err)
	}
}

// BenchmarkReadThroughput reads a 200 MB file sequentially over SFTP
// (loopback, in-process server).
func BenchmarkReadThroughput(b *testing.B) {
	s := newServer(b)
	dir := b.TempDir()
	data := make([]byte, 200<<20)
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), data, 0o644); err != nil {
		b.Fatal(err)
	}
	f := newFS(b, s, dir)
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		fl, err := f.Open(context.Background(), "big.bin")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.CopyBuffer(struct{ io.Writer }{io.Discard}, struct{ io.Reader }{fl}, make([]byte, 32<<10)); err != nil {
			b.Fatal(err)
		}
		fl.Close()
	}
}
