package sources

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/vfs"
	"github.com/fs0ciety7000/aether-media/internal/vfs/agentfs"
	"github.com/fs0ciety7000/aether-media/internal/vfs/local"
	"github.com/fs0ciety7000/aether-media/internal/vfs/sftpfs"
)

// DefaultFactory builds the real backends.
type DefaultFactory struct {
	UserAgent string
	HTTP      *http.Client
}

func (f DefaultFactory) Local(root string) (vfs.FS, error) { return local.New(root) }

func (f DefaultFactory) agentConfig(cfg AgentConfig, sec Secrets, root string) agentfs.Config {
	return agentfs.Config{BaseURL: cfg.URL, Token: sec.Token, Share: cfg.Share, Root: root, HTTPClient: f.HTTP, UserAgent: f.UserAgent}
}

func (f DefaultFactory) Agent(_ context.Context, cfg AgentConfig, sec Secrets, root string) (vfs.FS, error) {
	return agentfs.New(f.agentConfig(cfg, sec, root))
}

func (f DefaultFactory) sftpConfig(cfg SFTPConfig, sec Secrets, root string) sftpfs.Config {
	return sftpfs.Config{
		Host: cfg.Host, Port: cfg.Port, User: cfg.User, Password: sec.Password,
		PrivateKey: []byte(sec.PrivateKey), Passphrase: sec.Passphrase, HostKey: cfg.HostKey,
		Root: root, Timeout: 20 * time.Second,
	}
}

func (f DefaultFactory) SFTP(ctx context.Context, cfg SFTPConfig, sec Secrets, root string) (vfs.FS, error) {
	return sftpfs.New(ctx, f.sftpConfig(cfg, sec, root))
}

func (f DefaultFactory) SFTPHostKey(ctx context.Context, cfg SFTPConfig) (string, error) {
	fp, err := sftpfs.Probe(ctx, f.sftpConfig(cfg, Secrets{}, "/"))
	var hk *sftpfs.HostKeyError
	if errors.As(err, &hk) {
		return hk.Fingerprint, nil
	}
	return fp, err
}

func (f DefaultFactory) AgentInfo(ctx context.Context, url, token string) (AgentInfo, error) {
	info, err := agentfs.Info(ctx, url, token, f.HTTP)
	if err != nil {
		return AgentInfo{}, err
	}
	out := AgentInfo{Hostname: info.Hostname, Version: info.Version, Shares: []string{}}
	for _, s := range info.Shares {
		out.Shares = append(out.Shares, s.Name)
	}
	return out, nil
}

func (f DefaultFactory) AgentEvents(ctx context.Context, cfg AgentConfig, sec Secrets) (<-chan []string, error) {
	in, err := agentfs.Events(ctx, f.agentConfig(cfg, sec, ""))
	if err != nil {
		return nil, err
	}
	out := make(chan []string)
	go func() {
		defer close(out)
		for ev := range in {
			if ev.Share != "" && ev.Share != cfg.Share {
				continue
			}
			select {
			case out <- ev.Paths:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

var _ Factory = DefaultFactory{}
