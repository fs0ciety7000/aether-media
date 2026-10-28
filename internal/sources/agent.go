package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/vfs"
	"github.com/fs0ciety7000/aether-media/internal/vfs/agentfs"
)

// agentProbe asks an agent to run ffprobe on a file of its share
// (POST /agent/v1/probe, over the pinned transport) and returns the raw JSON.
func agentProbe(ctx context.Context, cfg AgentConfig, sec Secrets, rel string) ([]byte, error) {
	if cfg.Fingerprint != "" && !strings.HasPrefix(cfg.URL, "https://") {
		return nil, errors.New("agent épinglé joint en http:// : réappairez-le")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	q := url.Values{"share": {cfg.Share}, "path": {strings.TrimPrefix(rel, "/")}}
	u := strings.TrimSuffix(cfg.URL, "/") + "/agent/v1/probe?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sec.Token)
	res, err := agentfs.ClientFor(cfg.URL, cfg.Fingerprint).Do(req)
	if err != nil {
		// The cause stays wrapped: the circuit breaker classifies it.
		return nil, fmt.Errorf("%w: agent injoignable: %w", vfs.ErrOffline, err)
	}
	defer drainClose(res)
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

// drainClose reads the rest of a response (bounded) and closes it, so that
// its connection is reused.
func drainClose(res *http.Response) {
	agentfs.DrainClose(res.Body)
	_ = res.Body.Close() // already closed; spelled out for bodyclose
}
