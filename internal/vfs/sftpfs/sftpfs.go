// Package sftpfs implements vfs.FS over SFTP (seedboxes, NAS), with pinned
// host keys (TOFU confirmed by the user), keepalives and lazy reconnection.
package sftpfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

// Config describes an SFTP source.
type Config struct {
	Host       string
	Port       int // default 22
	User       string
	Password   string
	PrivateKey []byte // PEM (OpenSSH format accepted)
	Passphrase string
	HostKey    string // "SHA256:..." fingerprint, required
	Root       string // absolute remote path
	Timeout    time.Duration
}

// HostKeyError is returned when the server's host key is not pinned yet
// (Mismatch false: show Fingerprint to the user for confirmation) or differs
// from the pinned one (Mismatch true: possible spoofing).
type HostKeyError struct {
	Fingerprint string
	Mismatch    bool
}

func (e *HostKeyError) Error() string {
	if e.Mismatch {
		return "sftp: host key mismatch (server presented " + e.Fingerprint + "): possible spoofing"
	}
	return "sftp: host key unknown (" + e.Fingerprint + "): confirm the fingerprint"
}

// Tunables (variables for tests).
var (
	KeepaliveInterval = 30 * time.Second
	KeepaliveTimeout  = 15 * time.Second
)

const (
	defaultTimeout = 15 * time.Second
	maxPacket      = 32 << 10
	maxInflight    = 64
)

// ErrClosed is returned after Close.
var ErrClosed = errors.New("sftp: source closed")

type conn struct {
	ssh      *ssh.Client
	sftp     *sftp.Client
	root     string // configured root (lexical)
	realRoot string // root as resolved by the server
	done     chan struct{}
	once     sync.Once
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.sftp.Close()
		_ = c.ssh.Close()
	})
}

// FS is an SFTP source.
type FS struct {
	cfg  Config
	root string

	mu     sync.Mutex
	cur    *conn
	closed bool
}

var _ vfs.FS = (*FS)(nil)

func normFingerprint(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "SHA256:")
	return "SHA256:" + strings.TrimRight(s, "=")
}

func validate(cfg *Config) error {
	if cfg.Host == "" {
		return errors.New("sftp: host is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return errors.New("sftp: invalid port")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return nil
}

// New connects to the server (verifying the host key) and returns the source.
// A missing or different host key yields *HostKeyError.
func New(ctx context.Context, cfg Config) (vfs.FS, error) {
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	if cfg.User == "" {
		return nil, errors.New("sftp: user is required")
	}
	if cfg.Password == "" && len(cfg.PrivateKey) == 0 {
		return nil, errors.New("sftp: a password or a private key is required")
	}
	root := path.Clean(cfg.Root)
	if !strings.HasPrefix(root, "/") || strings.ContainsRune(root, 0) {
		return nil, fmt.Errorf("%w: sftp root must be an absolute path", vfs.ErrPath)
	}
	if _, err := authMethods(cfg); err != nil {
		return nil, err
	}
	f := &FS{cfg: cfg, root: root}
	if _, err := f.getConn(ctx); err != nil {
		var hk *HostKeyError
		if errors.As(err, &hk) || errors.Is(err, ErrAuth) {
			return nil, err
		}
		// Offline at startup is fine: the source reconnects lazily.
	}
	return f, nil
}

// ErrAuth is returned when the server rejects the credentials.
var ErrAuth = errors.New("sftp: authentication failed")

func authMethods(cfg Config) ([]ssh.AuthMethod, error) {
	var am []ssh.AuthMethod
	if len(cfg.PrivateKey) > 0 {
		var signer ssh.Signer
		var err error
		if cfg.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(cfg.PrivateKey, []byte(cfg.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(cfg.PrivateKey)
		}
		if err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return nil, errors.New("sftp: private key is encrypted: passphrase required")
			}
			return nil, fmt.Errorf("sftp: invalid private key: %w", err)
		}
		am = append(am, ssh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		pw := cfg.Password
		am = append(am, ssh.Password(pw),
			ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
				ans := make([]string, len(qs))
				for i := range ans {
					ans[i] = pw
				}
				return ans, nil
			}))
	}
	return am, nil
}

// handshake dials and runs the SSH handshake; hostKey is the callback.
func handshake(ctx context.Context, cfg Config, user string, auth []ssh.AuthMethod, hostKey ssh.HostKeyCallback) (*ssh.Client, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	d := net.Dialer{Timeout: cfg.Timeout, KeepAlive: 30 * time.Second}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: sftp: %w", vfs.ErrOffline, err)
	}
	_ = nc.SetDeadline(time.Now().Add(cfg.Timeout))
	stop := context.AfterFunc(ctx, func() { _ = nc.Close() })
	defer stop()
	sc, chans, reqs, err := ssh.NewClientConn(nc, addr, &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         cfg.Timeout,
	})
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	_ = nc.SetDeadline(time.Time{})
	return ssh.NewClient(sc, chans, reqs), nil
}

func (f *FS) dial(ctx context.Context) (*conn, error) {
	auth, err := authMethods(f.cfg)
	if err != nil {
		return nil, err
	}
	var hkErr *HostKeyError
	want := ""
	if f.cfg.HostKey != "" {
		want = normFingerprint(f.cfg.HostKey)
	}
	cb := func(_ string, _ net.Addr, key ssh.PublicKey) error {
		got := ssh.FingerprintSHA256(key)
		switch {
		case want == "":
			hkErr = &HostKeyError{Fingerprint: got}
		case normFingerprint(got) != want:
			hkErr = &HostKeyError{Fingerprint: got, Mismatch: true}
		default:
			return nil
		}
		return hkErr
	}
	sc, err := handshake(ctx, f.cfg, f.cfg.User, auth, cb)
	if err != nil {
		switch {
		case hkErr != nil:
			return nil, hkErr
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case errors.Is(err, vfs.ErrOffline):
			return nil, err
		case strings.Contains(err.Error(), "unable to authenticate"):
			return nil, fmt.Errorf("%w: %w", ErrAuth, err)
		}
		return nil, fmt.Errorf("%w: sftp: %w", vfs.ErrOffline, err)
	}
	cl, err := sftp.NewClient(sc,
		sftp.UseConcurrentReads(true),
		sftp.MaxPacket(maxPacket),
		sftp.MaxConcurrentRequestsPerFile(maxInflight),
	)
	if err != nil {
		_ = sc.Close()
		return nil, fmt.Errorf("%w: sftp subsystem: %w", vfs.ErrOffline, err)
	}
	c := &conn{ssh: sc, sftp: cl, root: f.root, realRoot: f.root, done: make(chan struct{})}
	if rp, err := cl.RealPath(f.root); err == nil && strings.HasPrefix(rp, "/") {
		c.realRoot = path.Clean(rp)
	}
	go f.watch(c)
	return c, nil
}

// watch drops c when the SSH connection dies and sends keepalives.
func (f *FS) watch(c *conn) {
	dead := make(chan struct{})
	go func() {
		_ = c.ssh.Wait()
		close(dead)
	}()
	t := time.NewTicker(KeepaliveInterval)
	defer t.Stop()
	for {
		select {
		case <-dead:
			f.drop(c)
			return
		case <-c.done:
			return
		case <-t.C:
			res := make(chan error, 1)
			go func() {
				_, _, err := c.ssh.SendRequest("keepalive@openssh.com", true, nil)
				res <- err
			}()
			kt := time.NewTimer(KeepaliveTimeout)
			select {
			case err := <-res:
				kt.Stop()
				if err != nil {
					f.drop(c)
					return
				}
			case <-kt.C:
				f.drop(c)
				return
			case <-c.done:
				kt.Stop()
				return
			}
		}
	}
}

// drop forgets c (if current) and closes it; the next call reconnects.
func (f *FS) drop(c *conn) {
	f.mu.Lock()
	if f.cur == c {
		f.cur = nil
	}
	f.mu.Unlock()
	c.close()
}

func (f *FS) getConn(ctx context.Context) (*conn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, ErrClosed
	}
	if f.cur != nil {
		return f.cur, nil
	}
	c, err := f.dial(ctx)
	if err != nil {
		return nil, err
	}
	f.cur = c
	return c, nil
}

// isConnErr reports whether err means the connection is unusable (as
// opposed to a per-file error reported by the server).
func isConnErr(err error) bool {
	if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, vfs.ErrPath) {
		return false
	}
	if errors.Is(err, sftp.ErrSSHFxConnectionLost) || errors.Is(err, sftp.ErrSSHFxNoConnection) {
		return true
	}
	var se *sftp.StatusError
	return !errors.As(err, &se)
}

// do runs op on a live connection, mapping connection failures to
// vfs.ErrOffline (and dropping the connection for the next call).
func (f *FS) do(ctx context.Context, op func(*conn) error) error {
	c, err := f.getConn(ctx)
	if err != nil {
		return err
	}
	err = op(c)
	if err != nil && isConnErr(err) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		f.drop(c)
		return fmt.Errorf("%w: sftp: %w", vfs.ErrOffline, err)
	}
	return err
}

// full maps a source-relative path to an absolute remote path.
func (f *FS) full(name string) (rel, full string, err error) {
	rel, err = vfs.CleanRel(name)
	if err != nil {
		return "", "", err
	}
	full = path.Join(f.root, rel)
	if !within(f.root, full) {
		return "", "", fmt.Errorf("%w: %q escapes root", vfs.ErrPath, name)
	}
	return rel, full, nil
}

func within(root, p string) bool {
	return p == root || root == "/" || strings.HasPrefix(p, root+"/")
}

// confined checks that p does not resolve outside the root: through the
// server's realpath (resolves symlinks on OpenSSH), plus an explicit check
// of a symlink in the final component for servers whose realpath is lexical.
func (c *conn) confined(p string) error {
	rp, err := c.sftp.RealPath(p)
	if err != nil {
		return err
	}
	if rp = path.Clean(rp); !within(c.realRoot, rp) && !within(c.root, rp) {
		return fmt.Errorf("%w: %s resolves outside the root", vfs.ErrPath, p)
	}
	fi, err := c.sftp.Lstat(p)
	if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		return err
	}
	target, err := c.sftp.ReadLink(p)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(target, "/") {
		target = path.Join(path.Dir(p), target)
	}
	if t := path.Clean(target); !within(c.realRoot, t) && !within(c.root, t) {
		return fmt.Errorf("%w: %s links outside the root", vfs.ErrPath, p)
	}
	return nil
}

// linkOK checks a symlink found while listing does not point outside.
func (c *conn) linkOK(full string) bool {
	return c.confined(full) == nil
}

func entry(rel string, fi os.FileInfo) vfs.Entry {
	e := vfs.Entry{Path: rel, ModTime: fi.ModTime(), IsDir: fi.IsDir()}
	if !e.IsDir {
		e.Size = fi.Size()
	}
	return e
}

// listed is a directory child; descend is false for symlinked directories.
type listed struct {
	vfs.Entry
	descend bool
}

// list reads dir and converts its children, resolving in-root symlinks.
func (c *conn) list(ctx context.Context, rel, full string) ([]listed, error) {
	fis, err := c.sftp.ReadDirContext(ctx, full)
	if err != nil {
		return nil, err
	}
	sort.Slice(fis, func(i, j int) bool { return fis[i].Name() < fis[j].Name() })
	out := make([]listed, 0, len(fis))
	for _, fi := range fis {
		name := fi.Name()
		if name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
			continue
		}
		p := path.Join(full, name)
		descend := true
		if fi.Mode()&fs.ModeSymlink != 0 {
			if !c.linkOK(p) {
				continue
			}
			st, err := c.sftp.Stat(p)
			if err != nil {
				if isConnErr(err) {
					return nil, err
				}
				continue
			}
			fi, descend = st, false
		}
		if !fi.IsDir() && !fi.Mode().IsRegular() {
			continue
		}
		out = append(out, listed{entry(vfs.Join(rel, name), fi), descend && fi.IsDir()})
	}
	return out, nil
}

// Walk lists entries below dir (dir excluded), parents before children.
// Unreadable sub-directories are skipped; symlinked directories are listed
// but not descended into.
func (f *FS) Walk(ctx context.Context, dir string, fn func(vfs.Entry) error) error {
	rel, full, err := f.full(dir)
	if err != nil {
		return err
	}
	if rel != "" {
		if err := f.do(ctx, func(c *conn) error { return c.confined(full) }); err != nil {
			return err
		}
	}
	stack := []string{rel}
	first := true
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		var es []listed
		err := f.do(ctx, func(c *conn) error {
			var err error
			es, err = c.list(ctx, cur, path.Join(f.root, cur))
			return err
		})
		if err != nil {
			if first || errors.Is(err, vfs.ErrOffline) || ctx.Err() != nil {
				return err
			}
			continue // unreadable sub-directory
		}
		first = false
		var subdirs []string
	entries:
		for _, e := range es {
			if err := fn(e.Entry); err != nil {
				switch {
				case errors.Is(err, fs.SkipAll):
					return nil
				case errors.Is(err, fs.SkipDir) && e.IsDir:
					continue
				case errors.Is(err, fs.SkipDir):
					// On a file: skip the rest of the parent directory.
					subdirs = nil
					break entries
				default:
					return err
				}
			}
			if e.descend {
				subdirs = append(subdirs, e.Path)
			}
		}
		for i := len(subdirs) - 1; i >= 0; i-- {
			stack = append(stack, subdirs[i])
		}
	}
	return nil
}

// Stat describes name.
func (f *FS) Stat(ctx context.Context, name string) (vfs.Entry, error) {
	rel, full, err := f.full(name)
	if err != nil {
		return vfs.Entry{}, err
	}
	var e vfs.Entry
	err = f.do(ctx, func(c *conn) error {
		if rel != "" {
			if err := c.confined(full); err != nil {
				return err
			}
		}
		fi, err := c.sftp.Stat(full)
		if err != nil {
			return err
		}
		e = entry(rel, fi)
		return nil
	})
	return e, err
}

// ReadDir lists the direct children of dir, sorted by name.
func (f *FS) ReadDir(ctx context.Context, dir string) ([]vfs.Entry, error) {
	rel, full, err := f.full(dir)
	if err != nil {
		return nil, err
	}
	var es []vfs.Entry
	err = f.do(ctx, func(c *conn) error {
		if rel != "" {
			if err := c.confined(full); err != nil {
				return err
			}
		}
		ls, err := c.list(ctx, rel, full)
		for _, l := range ls {
			es = append(es, l.Entry)
		}
		return err
	})
	return es, err
}

// InputURL returns "": the server proxies SFTP files to FFmpeg over HTTP.
func (f *FS) InputURL(string) string { return "" }

// Ping stats the root.
func (f *FS) Ping(ctx context.Context) error {
	return f.do(ctx, func(c *conn) error {
		fi, err := c.sftp.Stat(f.root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%w: sftp root %s does not exist", vfs.ErrOffline, f.root)
			}
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("%w: sftp root %s is not a directory", vfs.ErrOffline, f.root)
		}
		return nil
	})
}

// Close closes the connection; the source cannot be used afterwards.
func (f *FS) Close() error {
	f.mu.Lock()
	c := f.cur
	f.cur, f.closed = nil, true
	f.mu.Unlock()
	if c != nil {
		c.close()
	}
	return nil
}

var errProbeDone = errors.New("sftp: probe done")

// Probe connects only far enough to obtain the server's host key fingerprint
// ("SHA256:..."), for the user confirmation flow. No authentication is made.
func Probe(ctx context.Context, cfg Config) (string, error) {
	if err := validate(&cfg); err != nil {
		return "", err
	}
	user := cfg.User
	if user == "" {
		user = "aether-probe"
	}
	var fp string
	cb := func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fp = ssh.FingerprintSHA256(key)
		return errProbeDone
	}
	c, err := handshake(ctx, cfg, user, nil, cb)
	if c != nil {
		_ = c.Close()
	}
	if fp != "" {
		return fp, nil
	}
	if err == nil {
		err = errors.New("sftp: no host key received")
	}
	return "", err
}
