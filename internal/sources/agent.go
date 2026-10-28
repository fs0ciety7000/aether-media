package sources

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
)

var agentHTTP = &http.Client{Timeout: 90 * time.Second}

// agentProbe asks an agent to run ffprobe on a file of its share
// (POST /agent/v1/probe) and returns the raw JSON.
func agentProbe(ctx context.Context, cfg AgentConfig, sec Secrets, rel string) ([]byte, error) {
	q := url.Values{"share": {cfg.Share}, "path": {strings.TrimPrefix(rel, "/")}}
	u := strings.TrimSuffix(cfg.URL, "/") + "/agent/v1/probe?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sec.Token)
	res, err := agentHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: agent injoignable: %v", vfs.ErrOffline, err) //nolint:errorlint // keep a single sentinel
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("%w: %s", probe.ErrNotMedia, strings.TrimSpace(string(body)))
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("agent probe: HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
