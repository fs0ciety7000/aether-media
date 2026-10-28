// Package events is an in-process publish/subscribe hub used to push server
// events (scan progress, new items...) to web clients over SSE.
package events

import (
	"sync"
	"time"
)

// Event is a typed message. Data must be JSON-serialisable.
type Event struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
	Data any       `json:"data,omitempty"`
}

// Hub fans events out to subscribers. Slow subscribers drop events rather
// than blocking publishers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewHub() *Hub { return &Hub{subs: make(map[chan Event]struct{})} }

// Publish sends an event to every subscriber without blocking.
func (h *Hub) Publish(typ string, data any) {
	ev := Event{Type: typ, Time: time.Now().UTC(), Data: data}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Subscribe returns a channel of events and a function to unsubscribe.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}
