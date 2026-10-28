package syncsvc

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/store"
)

// ---- matching ----

type epKey struct {
	series          int64
	season, episode int
}

// index maps remote references to local items (by the provider ids Aether
// already stores) and local items back to references.
type index struct {
	items   map[int64]store.SyncItem
	idsOf   map[int64]IDs // movie / series ids
	movies  map[string]int64
	shows   map[string]int64
	eps     map[epKey]int64
	refsMem map[int64][]Ref
}

func idKey(kind, value string) string { return kind + ":" + value }

func parseIDs(raw string) IDs {
	var m map[string]string
	if json.Unmarshal([]byte(raw), &m) != nil {
		return IDs{}
	}
	return IDs{Tmdb: m["tmdb"], Imdb: m["imdb"], Tvdb: m["tvdb"]}
}

func newIndex(items []store.SyncItem) *index {
	ix := &index{items: map[int64]store.SyncItem{}, idsOf: map[int64]IDs{}, movies: map[string]int64{},
		shows: map[string]int64{}, eps: map[epKey]int64{}, refsMem: map[int64][]Ref{}}
	for _, it := range items {
		ix.items[it.ID] = it
		switch it.Kind {
		case "movie", "series":
			ids := parseIDs(it.ProviderIDs)
			ix.idsOf[it.ID] = ids
			m := ix.movies
			if it.Kind == "series" {
				m = ix.shows
			}
			for k, v := range map[string]string{"tmdb": ids.Tmdb, "imdb": ids.Imdb, "tvdb": ids.Tvdb} {
				if v != "" {
					if _, dup := m[idKey(k, v)]; !dup {
						m[idKey(k, v)] = it.ID
					}
				}
			}
		case "episode":
			if it.SeriesID == 0 || it.Season < 0 || it.Episode < 0 {
				continue
			}
			last := max(it.EpisodeEnd, it.Episode)
			for n := it.Episode; n <= last && n-it.Episode < 50; n++ {
				ix.eps[epKey{it.SeriesID, int(it.Season), int(n)}] = it.ID
			}
		}
	}
	return ix
}

func lookup(m map[string]int64, ids IDs, order []string) (int64, bool) {
	for _, k := range order {
		v := map[string]string{"tmdb": ids.Tmdb, "imdb": ids.Imdb, "tvdb": ids.Tvdb}[k]
		if v == "" {
			continue
		}
		if id, ok := m[idKey(k, v)]; ok {
			return id, true
		}
	}
	return 0, false
}

// match finds the local item of a remote reference.
func (ix *index) match(r Ref) (int64, bool) {
	switch r.Kind {
	case "movie":
		return lookup(ix.movies, r.IDs, []string{"tmdb", "imdb", "tvdb"})
	case "show":
		return lookup(ix.shows, r.IDs, []string{"tvdb", "tmdb", "imdb"})
	case "episode":
		show, ok := lookup(ix.shows, r.IDs, []string{"tvdb", "tmdb", "imdb"})
		if !ok {
			return 0, false
		}
		id, ok := ix.eps[epKey{show, r.Season, r.Episode}]
		return id, ok
	}
	return 0, false
}

// refs returns the references of a local item (several for a multi-episode
// file); nil when it carries no usable id.
func (ix *index) refs(itemID int64) []Ref {
	if r, ok := ix.refsMem[itemID]; ok {
		return r
	}
	it, ok := ix.items[itemID]
	var out []Ref
	if ok {
		switch it.Kind {
		case "movie":
			if ids := ix.idsOf[itemID]; !ids.empty() {
				out = []Ref{{Kind: "movie", IDs: ids}}
			}
		case "series":
			if ids := ix.idsOf[itemID]; !ids.empty() {
				out = []Ref{{Kind: "show", IDs: ids}}
			}
		case "episode":
			if ids := ix.idsOf[it.SeriesID]; !ids.empty() && it.Season >= 0 && it.Episode >= 0 {
				for n := it.Episode; n <= max(it.EpisodeEnd, it.Episode) && n-it.Episode < 50; n++ {
					out = append(out, Ref{Kind: "episode", IDs: ids, Season: int(it.Season), Episode: int(n)})
				}
			}
		}
	}
	ix.refsMem[itemID] = out
	return out
}

// refKind is the reference kind of a local item.
func (ix *index) refKind(itemID int64) string {
	switch ix.items[itemID].Kind {
	case "movie":
		return "movie"
	case "series":
		return "show"
	case "episode":
		return "episode"
	}
	return ""
}

// ---- three-way merge ----

// state is one side's view of an item for a feature.
type state struct {
	present bool
	at      time.Time // when it was set (or removed, when !present)
	value   float64
	ref     string // remote entry id
}

type conflict struct {
	itemID int64
	winner string // local | remote
}

// plan is what a merge decided.
type plan struct {
	pushAdd     map[int64]state // to send to the service
	pushRemove  map[int64]state // to remove from the service (ref = remote id)
	localSet    map[int64]state // to write locally
	localRemove []int64
	shadow      map[int64]state // state of both sides once applied
	conflicts   []conflict
}

// merge compares the local state, the remote state and the state recorded
// after the previous synchronisation (shadow). A change on one side only is
// copied to the other; a change on both sides is settled by date (most
// recent wins; a remote removal is dated removedAt). local may hold absent
// entries carrying the removal date; remote and shadow only hold present ones.
func merge(local, remote, shadow map[int64]state, removedAt time.Time, eq func(a, b float64) bool) plan {
	if eq == nil {
		eq = func(float64, float64) bool { return true }
	}
	p := plan{pushAdd: map[int64]state{}, pushRemove: map[int64]state{}, localSet: map[int64]state{}, shadow: map[int64]state{}}
	keys := map[int64]bool{}
	for k, v := range local {
		if v.present {
			keys[k] = true
		}
	}
	for k := range remote {
		keys[k] = true
	}
	for k := range shadow {
		keys[k] = true
	}
	for k := range keys {
		l, r := local[k], remote[k]
		sh, inS := shadow[k]
		inL, inR := l.present, r.present
		lc := inL != inS || (inL && inS && !eq(l.value, sh.value))
		rc := inR != inS || (inR && inS && !eq(r.value, sh.value))
		localWins := func() {
			if inL {
				p.pushAdd[k] = l
				p.shadow[k] = state{present: true, at: l.at, value: l.value, ref: firstNonEmpty(r.ref, sh.ref)}
			} else if inR {
				p.pushRemove[k] = r
			}
		}
		remoteWins := func() {
			if inR {
				p.localSet[k] = r
				p.shadow[k] = r
			} else {
				p.localRemove = append(p.localRemove, k)
			}
		}
		switch {
		case !lc && !rc:
			if inR {
				p.shadow[k] = r
			}
		case lc && !rc:
			localWins()
		case !lc && rc:
			remoteWins()
		case inL == inR && (!inL || eq(l.value, r.value)):
			// Same change on both sides.
			if inR {
				p.shadow[k] = r
			}
		default:
			tr := r.at
			if !inR {
				tr = removedAt
			}
			if l.at.After(tr) || (l.at.Equal(tr) && inL) {
				localWins()
				p.conflicts = append(p.conflicts, conflict{k, "local"})
			} else {
				remoteWins()
				p.conflicts = append(p.conflicts, conflict{k, "remote"})
			}
		}
	}
	return p
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func ratingEqual(a, b float64) bool   { return math.Round(a) == math.Round(b) }
func progressEqual(a, b float64) bool { return math.Abs(a-b) < 2 }

// toRemoteRating maps Aether's 0-10 scale onto 1-10 integers.
func toRemoteRating(v float64) float64 { return math.Min(10, math.Max(1, math.Round(v))) }

// unix converts a timestamp column (0 = unknown).
func unix(t int64) time.Time {
	if t <= 0 {
		return time.Time{}
	}
	return time.Unix(t, 0).UTC()
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
