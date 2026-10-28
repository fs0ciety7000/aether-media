package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/images"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// T-047: actor photos go through the image cache. The first request for a
// person downloads the remote photo (SSRF-guarded client of the cache), keeps
// its content hash in people.image_hash, and every request is then served
// from the cache like any other artwork (resized, immutable).

// imageIngester is implemented by *images.Cache.
type imageIngester interface {
	Ingest(ctx context.Context, src string) (images.Source, error)
}

// personFailures remembers people whose photo could not be fetched, so a
// broken URL is not downloaded again on every page view.
type personFailures struct {
	mu sync.Mutex
	at map[int64]time.Time
}

const personRetryAfter = time.Hour

func (f *personFailures) recent(id int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.at[id]
	if ok && time.Since(t) > personRetryAfter {
		delete(f.at, id)
		return false
	}
	return ok
}

func (f *personFailures) add(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.at == nil || len(f.at) > 10000 {
		f.at = map[int64]time.Time{}
	}
	f.at[id] = time.Now()
}

// remotePhoto reports whether a person's image source can be fetched.
func remotePhoto(u string) bool {
	return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")
}

// personImageURL is the API URL of a person's photo, versioned by its
// source so that browsers may cache it forever; "" when there is none.
func personImageURL(id int64, source string) string {
	if !remotePhoto(source) {
		return ""
	}
	sum := sha256.Sum256([]byte(source))
	return "/api/v1/people/" + strconv.FormatInt(id, 10) + "/image?v=" + hex.EncodeToString(sum[:4])
}

// personImage: GET /api/v1/people/{id}/image?w=185
func (s *Server) personImage(w http.ResponseWriter, r *http.Request) {
	ing, ok := s.Images.(imageIngester)
	if s.Images == nil || !ok {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := s.store.GetPersonImage(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	hash := p.ImageHash
	if hash == "" {
		if !remotePhoto(p.ImageUrl) || s.personFails.recent(id) {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		src, err := ing.Ingest(ctx, p.ImageUrl)
		cancel()
		if err != nil {
			if r.Context().Err() == nil {
				s.personFails.add(id)
				s.log.Warn("photo de personne indisponible", "person", id, "err", err)
			}
			http.NotFound(w, r)
			return
		}
		hash = src.Hash
		if err := s.store.SetPersonImageHash(r.Context(), db.SetPersonImageHashParams{ImageHash: hash, ID: id, ImageUrl: p.ImageUrl}); err != nil {
			s.log.Warn("photo de personne non enregistrée", "person", id, "err", err)
		}
	}
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))
	s.Images.ServeVariant(w, r, hash, width)
}
