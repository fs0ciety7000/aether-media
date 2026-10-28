// Package logbuf keeps the most recent log records in memory for the admin
// UI (filter, search, live tail, download). It is plugged into slog as an
// additional handler; secrets are masked before anything is stored.
package logbuf

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// DefaultCapacity is the number of records kept by Default.
const DefaultCapacity = 2000

// Default is the process-wide buffer wired by the server's logger setup.
var Default = New(DefaultCapacity)

// Attr is one key/value pair of a record (groups flattened as "a.b").
type Attr struct {
	Key   string `json:"k"`
	Value string `json:"v"`
}

// Entry is a stored log record.
type Entry struct {
	Seq   int64     `json:"seq"`
	Time  time.Time `json:"time"`
	Level string    `json:"level"` // DEBUG | INFO | WARN | ERROR
	Msg   string    `json:"msg"`
	Attrs []Attr    `json:"attrs,omitempty"`

	level slog.Level
}

// LevelValue returns the slog level of the entry.
func (e Entry) LevelValue() slog.Level { return e.level }

// Buffer is a fixed-size ring of entries with live subscribers.
type Buffer struct {
	mu      sync.Mutex
	ring    []Entry
	next    int
	full    bool
	seq     int64
	subs    map[chan Entry]struct{}
	dropped int64 // entries not delivered to slow subscribers
}

// New returns a buffer keeping the last capacity entries.
func New(capacity int) *Buffer {
	return &Buffer{ring: make([]Entry, max(1, capacity)), subs: map[chan Entry]struct{}{}}
}

// Capacity returns the maximum number of entries kept.
func (b *Buffer) Capacity() int { return len(b.ring) }

func (b *Buffer) add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	e.Seq = b.seq
	b.ring[b.next] = e
	b.next = (b.next + 1) % len(b.ring)
	if b.next == 0 {
		b.full = true
	}
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
			b.dropped++
		}
	}
}

// Filter selects entries.
type Filter struct {
	MinLevel slog.Level // entries below are skipped (the zero value is INFO)
	Query    string     // case-insensitive substring of the message or an attribute
	After    int64      // only entries with Seq > After
	Limit    int        // keep the most recent Limit entries (0 = all)
}

// Match reports whether e passes the filter (Limit aside).
func (f Filter) Match(e Entry) bool {
	if e.level < f.MinLevel || e.Seq <= f.After {
		return false
	}
	if f.Query == "" {
		return true
	}
	q := strings.ToLower(f.Query)
	if strings.Contains(strings.ToLower(e.Msg), q) {
		return true
	}
	for _, a := range e.Attrs {
		if strings.Contains(strings.ToLower(a.Key), q) || strings.Contains(strings.ToLower(a.Value), q) {
			return true
		}
	}
	return false
}

// Entries returns the matching entries, oldest first, and the total number
// of entries currently held.
func (b *Buffer) Entries(f Filter) ([]Entry, int) {
	b.mu.Lock()
	var all []Entry
	if b.full {
		all = append(all, b.ring[b.next:]...)
	}
	all = append(all, b.ring[:b.next]...)
	b.mu.Unlock()
	out := make([]Entry, 0, len(all))
	for _, e := range all {
		if f.Match(e) {
			out = append(out, e)
		}
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[len(out)-f.Limit:]
	}
	return out, len(all)
}

// Subscribe returns a channel receiving every new entry (dropped when the
// reader is too slow) and a function to unsubscribe.
func (b *Buffer) Subscribe() (<-chan Entry, func()) {
	ch := make(chan Entry, 256)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// Format renders an entry as one text line (download).
func Format(e Entry) string {
	var sb strings.Builder
	sb.WriteString(e.Time.Format("2006-01-02T15:04:05.000Z07:00"))
	sb.WriteByte(' ')
	fmt.Fprintf(&sb, "%-5s ", e.Level)
	sb.WriteString(e.Msg)
	for _, a := range e.Attrs {
		sb.WriteByte(' ')
		sb.WriteString(a.Key)
		sb.WriteByte('=')
		if strings.ContainsAny(a.Value, " \t\n\"=") || a.Value == "" {
			fmt.Fprintf(&sb, "%q", a.Value)
		} else {
			sb.WriteString(a.Value)
		}
	}
	return sb.String()
}

// LevelName is the stored name of a level.
func LevelName(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "DEBUG"
	case l < slog.LevelWarn:
		return "INFO"
	case l < slog.LevelError:
		return "WARN"
	}
	return "ERROR"
}

// ParseLevel reads "debug", "info", "warn" or "error" (default: debug, i.e. all).
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelDebug
}

// ---- slog.Handler ----

// Handler returns a slog handler storing records at or above level.
func (b *Buffer) Handler(level slog.Leveler) slog.Handler {
	if level == nil {
		level = slog.LevelInfo
	}
	return &handler{buf: b, level: level}
}

type handler struct {
	buf    *Buffer
	level  slog.Leveler
	attrs  []Attr
	prefix string // open groups, "a.b."
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	e := Entry{Time: r.Time.UTC(), level: r.Level, Level: LevelName(r.Level), Msg: Redact(r.Message)}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	e.Attrs = append(e.Attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		e.Attrs = appendAttr(e.Attrs, h.prefix, a)
		return true
	})
	h.buf.add(e)
	return nil
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	n := *h
	n.attrs = append([]Attr(nil), h.attrs...)
	for _, a := range as {
		n.attrs = appendAttr(n.attrs, h.prefix, a)
	}
	return &n
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := *h
	n.prefix = h.prefix + name + "."
	return &n
}

func appendAttr(dst []Attr, prefix string, a slog.Attr) []Attr {
	v := a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return dst
	}
	if v.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, g := range v.Group() {
			dst = appendAttr(dst, p, g)
		}
		return dst
	}
	key := prefix + a.Key
	var s string
	switch v.Kind() {
	case slog.KindTime:
		s = v.Time().UTC().Format(time.RFC3339)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok && err != nil {
			s = err.Error()
		} else {
			s = fmt.Sprint(v.Any())
		}
	default:
		s = v.String()
	}
	if sensitiveKey(a.Key) {
		s = masked
	} else {
		s = Redact(s)
	}
	return append(dst, Attr{Key: key, Value: s})
}
