package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// events streams server events as Server-Sent Events.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "événements indisponibles")
		return
	}
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 5000\n\n")
	if err := rc.Flush(); err != nil {
		return
	}

	ch, unsubscribe := s.Hub.Subscribe()
	defer unsubscribe()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": ping\n\n")
		case ev := <-ch:
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
