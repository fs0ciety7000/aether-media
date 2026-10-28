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

	// Audience: UserID > 0 limits the event to that user (and is never
	// sent to anyone else, admins included); AdminOnly limits it to admins.
	UserID    int64 `json:"-"`
	AdminOnly bool  `json:"-"`
}

// VisibleTo reports whether a subscriber may receive the event.
func (e Event) VisibleTo(userID int64, admin bool) bool {
	if e.UserID > 0 {
		return e.UserID == userID
	}
	return !e.AdminOnly || admin
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
	h.send(Event{Type: typ, Time: time.Now().UTC(), Data: data})
}

// PublishUser sends an event to the given user's subscribers only.
func (h *Hub) PublishUser(userID int64, typ string, data any) {
	h.send(Event{Type: typ, Time: time.Now().UTC(), Data: data, UserID: userID})
}

// PublishAdmin sends an event to administrators only.
func (h *Hub) PublishAdmin(typ string, data any) {
	h.send(Event{Type: typ, Time: time.Now().UTC(), Data: data, AdminOnly: true})
}

func (h *Hub) send(ev Event) {
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
