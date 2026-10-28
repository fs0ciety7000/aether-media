package events

import (
	"testing"
	"time"
)

func TestAudience(t *testing.T) {
	h := NewHub()
	ch, stop := h.Subscribe()
	defer stop()
	h.Publish("all", nil)
	h.PublishUser(7, "user", nil)
	h.PublishAdmin("admin", nil)

	type sub struct {
		id    int64
		admin bool
		want  []string
	}
	evs := make([]Event, 0, 3)
	for range 3 {
		select {
		case ev := <-ch:
			evs = append(evs, ev)
		case <-time.After(time.Second):
			t.Fatal("event missing")
		}
	}
	for _, s := range []sub{
		{7, false, []string{"all", "user"}},
		{8, false, []string{"all"}},
		{8, true, []string{"all", "admin"}},
		{7, true, []string{"all", "user", "admin"}},
	} {
		var got []string
		for _, ev := range evs {
			if ev.VisibleTo(s.id, s.admin) {
				got = append(got, ev.Type)
			}
		}
		if len(got) != len(s.want) {
			t.Fatalf("user %d admin=%v: got %v, want %v", s.id, s.admin, got, s.want)
		}
		for i := range got {
			if got[i] != s.want[i] {
				t.Fatalf("user %d admin=%v: got %v, want %v", s.id, s.admin, got, s.want)
			}
		}
	}
}
