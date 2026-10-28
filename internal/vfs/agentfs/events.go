package agentfs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ChangeEvent reports changed paths of a share. When the subscription was
// created with a Root, Paths are relative to it (paths outside are dropped).
type ChangeEvent struct {
	Share string   `json:"share"`
	Paths []string `json:"paths"`
	// Resync is set on the synthetic event sent after a reconnection:
	// changes may have been missed while disconnected, so callers should
	// rescan the source.
	Resync bool `json:"-"`
}

// Backoff bounds for the SSE reconnect loop (variables for tests).
var (
	EventsMinBackoff = time.Second
	EventsMaxBackoff = 60 * time.Second
	// EventsIdleTimeout drops a stream that stays silent (the agent pings
	// every 25 s).
	EventsIdleTimeout = 90 * time.Second
)

// Events subscribes to /agent/v1/events and delivers change events for
// cfg.Share (all shares if cfg.Share is empty) until ctx is done, then closes
// the channel. Lost connections are re-established with exponential backoff;
// after each successful reconnection a ChangeEvent{Resync: true} is sent.
//
// An invalid configuration or a token rejected on the first attempt is
// returned as an error; an unreachable agent is not (it is retried).
func Events(ctx context.Context, cfg Config) (<-chan ChangeEvent, error) {
	share := cfg.Share
	cfg.Share = "-" // newFS requires one; filtering uses the real value
	f, err := newFS(cfg)
	if err != nil {
		return nil, err
	}
	f.share = share
	// Streams must not be cut by a client-wide timeout.
	if f.client.Timeout != 0 {
		c := *f.client
		c.Timeout = 0
		f.client = &c
	}
	resp, err := f.openEvents(ctx) //nolint:bodyclose // closed by eventLoop
	if err != nil && (errors.Is(err, ErrUnauthorized) || ctx.Err() != nil) {
		return nil, err
	}
	ch := make(chan ChangeEvent, 16)
	go f.eventLoop(ctx, resp, ch)
	return ch, nil
}

func (f *FS) openEvents(ctx context.Context) (*http.Response, error) {
	u := *f.base
	u.Path += apiPrefix + "/events"
	resp, err := f.do(ctx, http.MethodGet, u.String(), http.Header{"Accept": {"text/event-stream"}})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusErr(resp)
	}
	return resp, nil
}

func (f *FS) eventLoop(ctx context.Context, resp *http.Response, ch chan<- ChangeEvent) {
	defer close(ch)
	backoff := EventsMinBackoff
	first := true
	for {
		if resp != nil {
			if !first {
				select {
				case ch <- ChangeEvent{Share: f.share, Resync: true}:
				case <-ctx.Done():
					_ = resp.Body.Close()
					return
				}
			}
			first = false
			if f.readEvents(ctx, resp, ch) {
				backoff = EventsMinBackoff // the stream was healthy for a while
			}
			_ = resp.Body.Close()
		}
		if ctx.Err() != nil {
			return
		}
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		backoff = min(backoff*2, EventsMaxBackoff)
		var err error
		resp, err = f.openEvents(ctx)
		if err != nil {
			resp = nil
		}
	}
}

// readEvents consumes one SSE stream; it reports whether at least one
// message (event or ping) was received.
func (f *FS) readEvents(ctx context.Context, resp *http.Response, ch chan<- ChangeEvent) bool {
	// Idle watchdog: closing the body unblocks the scanner.
	activity := make(chan struct{}, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTimer(EventsIdleTimeout)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				_ = resp.Body.Close()
				return
			case <-activity:
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				t.Reset(EventsIdleTimeout)
			case <-t.C:
				_ = resp.Body.Close()
				return
			}
		}
	}()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), maxLineBytes)
	var event string
	var data strings.Builder
	got := false
	for sc.Scan() {
		select {
		case activity <- struct{}{}:
		default:
		}
		line := sc.Text()
		switch {
		case line == "":
			if data.Len() > 0 && (event == "" || event == "change") {
				if ev, ok := f.parseChange(data.String()); ok {
					select {
					case ch <- ev:
					case <-ctx.Done():
						return got
					}
				}
			}
			event = ""
			data.Reset()
		case strings.HasPrefix(line, ":"):
			got = true // ping / comment
		default:
			got = true
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				event = value
			case "data":
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(value)
			}
		}
	}
	return got
}

func (f *FS) parseChange(data string) (ChangeEvent, bool) {
	var ev ChangeEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return ev, false
	}
	if f.share != "" && ev.Share != f.share {
		return ev, false
	}
	if f.share == "" || f.root == "" {
		return ev, true
	}
	paths := ev.Paths[:0]
	for _, p := range ev.Paths {
		if lp, ok := f.local(p); ok {
			paths = append(paths, lp)
		}
	}
	if len(paths) == 0 {
		return ev, false
	}
	ev.Paths = paths
	return ev, true
}

// String implements fmt.Stringer (debug logging).
func (e ChangeEvent) String() string {
	if e.Resync {
		return fmt.Sprintf("%s: resync", e.Share)
	}
	return fmt.Sprintf("%s: %d path(s)", e.Share, len(e.Paths))
}
