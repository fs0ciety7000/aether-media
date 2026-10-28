package refresh

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/naming"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// Job kinds handled by the refresher.
const (
	JobRefresh = "metadata.refresh"
	JobImage   = "image.fetch"
)

// Identification statuses (items.identify_status).
const (
	StatusPending   = "pending"
	StatusMatched   = "matched"
	StatusUnmatched = "unmatched"
	StatusManual    = "manual"
	StatusLocal     = "local"
)

// ImageInfo is what the image cache reports after ingesting a source.
type ImageInfo struct {
	Hash          string
	Width, Height int
	Blurhash      string
	Dominant      string
}

// Ingester downloads/reads an image source into the cache. It must return an
// error wrapping jobs.ErrPermanent when retrying is pointless (404, not an
// image).
type Ingester func(ctx context.Context, src string) (ImageInfo, error)

// Enabler is implemented by providers that can be switched off (no API key).
type Enabler interface{ Enabled() bool }

// Config tunes identification.
type Config struct {
	Language string // e.g. "fr-FR"
	// MinScore is the confidence needed to accept a search hit automatically.
	MinScore float64
	// MinMargin is the lead the best hit needs over the runner-up.
	MinMargin float64
}

// Refresher identifies items and keeps their metadata and artwork current.
type Refresher struct {
	store  *store.Store
	jobs   *jobs.Runner
	hub    *events.Hub
	log    *slog.Logger
	cfg    Config
	local  []metadata.LocalProvider
	remote []metadata.RemoteProvider
	ingest Ingester
	lang   atomic.Pointer[string]
}

// SetLanguage changes the metadata language of future refreshes.
func (r *Refresher) SetLanguage(lang string) { r.lang.Store(&lang) }

// RefreshAll schedules every top-level item of every library (a refresh
// cascades to seasons, episodes and tracks). It returns the number queued.
func (r *Refresher) RefreshAll(ctx context.Context) (int, error) {
	libs, err := r.store.ListLibraries(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, l := range libs {
		ids, err := r.store.ListRefreshRoots(ctx, l.ID)
		if err != nil {
			return n, err
		}
		for _, id := range ids {
			ok, err := r.Enqueue(ctx, id, false)
			if err != nil {
				return n, err
			}
			if ok {
				n++
			}
		}
	}
	return n, nil
}

func New(s *store.Store, r *jobs.Runner, hub *events.Hub, log *slog.Logger, cfg Config,
	local []metadata.LocalProvider, remote []metadata.RemoteProvider, ingest Ingester) *Refresher {
	if cfg.MinScore == 0 {
		cfg.MinScore = 0.75
	}
	if cfg.MinMargin == 0 {
		cfg.MinMargin = 0.03
	}
	rf := &Refresher{store: s, jobs: r, hub: hub, log: log, cfg: cfg, local: local, remote: remote, ingest: ingest}
	rf.SetLanguage(cfg.Language)
	return rf
}

type refreshPayload struct {
	ItemID int64 `json:"itemId"`
	// Research forgets automatic matches and searches again.
	Research bool `json:"research,omitempty"`
}

type imagePayload struct {
	ImageID int64 `json:"imageId"`
}

// Enqueue schedules a refresh of one item. Seasons cover their episodes and
// series cascade to their seasons.
func (r *Refresher) Enqueue(ctx context.Context, itemID int64, research bool) (bool, error) {
	return r.jobs.Enqueue(ctx, JobRefresh, fmt.Sprint(itemID), refreshPayload{ItemID: itemID, Research: research}, 5)
}

// EnqueueLibrary schedules every item of a library that was never refreshed.
func (r *Refresher) EnqueueLibrary(ctx context.Context, libID int64) error {
	rows, err := r.store.ListItemsNeedingRefresh(ctx, libID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := r.Enqueue(ctx, row.ID, false); err != nil {
			return err
		}
	}
	if len(rows) > 0 {
		r.log.Info("métadonnées planifiées", "library", libID, "items", len(rows))
	}
	return nil
}

// HandleRefresh is the jobs.Handler for JobRefresh.
func (r *Refresher) HandleRefresh(ctx context.Context, j jobs.Job) error {
	var p refreshPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
	}
	it, err := r.store.GetItem(ctx, p.ItemID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := r.refresh(ctx, it, p.Research); err != nil {
		return err
	}
	r.hub.Publish("item.updated", map[string]any{"id": it.ID, "kind": it.Kind})
	return nil
}

func (r *Refresher) refresh(ctx context.Context, it db.Item, research bool) error {
	if it.Kind == metadata.KindTrack {
		return nil // tracks get their data from tags (probe)
	}
	q, err := r.query(ctx, it)
	if err != nil {
		return err
	}
	status := it.IdentifyStatus
	if research && status != StatusManual {
		delete(q.ProviderIDs, primaryID(it.Kind))
	}

	var layers []*metadata.Result
	for _, lp := range r.local {
		if !lp.Supports(it.Kind) {
			continue
		}
		res, err := lp.Local(ctx, q)
		if err != nil {
			r.log.Warn("métadonnées locales illisibles", "provider", lp.Name(), "item", it.ID, "err", err)
			continue
		}
		if res != nil {
			layers = append(layers, res)
			for k, v := range res.ProviderIDs {
				if _, ok := q.ProviderIDs[k]; !ok && v != "" {
					q.ProviderIDs[k] = v
				}
			}
		}
	}

	remote, status, err := r.fetchRemote(ctx, it, &q, status)
	if err != nil {
		return err
	}
	if remote != nil {
		layers = append(layers, remote)
	}
	if status == StatusPending {
		status = StatusLocal
	}
	merged := Merge(layers...)
	if err := r.save(ctx, it, q.ProviderIDs, merged, status); err != nil {
		return err
	}

	switch it.Kind {
	case metadata.KindSeries:
		seasons, err := r.store.ListChildItems(ctx, sql.NullInt64{Int64: it.ID, Valid: true})
		if err != nil {
			return err
		}
		for _, s := range seasons {
			if _, err := r.Enqueue(ctx, s.ID, false); err != nil {
				return err
			}
		}
	case metadata.KindSeason:
		return r.refreshEpisodes(ctx, it, q, merged)
	}
	return nil
}

// fetchRemote identifies the item if needed and fetches the first enabled
// remote provider supporting its kind.
func (r *Refresher) fetchRemote(ctx context.Context, it db.Item, q *metadata.Query, status string) (*metadata.Result, string, error) {
	for _, rp := range r.remote {
		if !rp.Supports(it.Kind) {
			continue
		}
		if e, ok := rp.(Enabler); ok && !e.Enabled() {
			continue
		}
		key := primaryID(it.Kind)
		if !identifiable(it.Kind) {
			// Seasons and episodes are fetched through their identified series.
			if q.ParentIDs[key] == "" {
				return nil, status, nil
			}
			status = StatusMatched
		} else if key != "" && q.ProviderIDs[key] == "" {
			cands, err := rp.Search(ctx, *q)
			if err != nil {
				return nil, status, r.remoteErr(rp, err)
			}
			best, ok := r.pick(cands)
			if !ok {
				r.log.Info("à identifier", "item", it.ID, "title", it.Title, "candidates", len(cands))
				return nil, StatusUnmatched, nil
			}
			for k, v := range best.ProviderIDs {
				q.ProviderIDs[k] = v
			}
			if status != StatusManual {
				status = StatusMatched
			}
		} else if status == StatusPending || status == StatusUnmatched || status == StatusLocal {
			status = StatusMatched // ids known (path, NFO or previous match)
		}
		res, err := rp.Fetch(ctx, *q)
		if errors.Is(err, metadata.ErrNotFound) {
			r.log.Warn("identifiant inconnu du fournisseur", "provider", rp.Name(), "item", it.ID, "ids", q.ProviderIDs)
			return nil, StatusUnmatched, nil
		}
		if err != nil {
			return nil, status, r.remoteErr(rp, err)
		}
		return res, status, nil
	}
	return nil, status, nil
}

func (r *Refresher) remoteErr(rp metadata.RemoteProvider, err error) error {
	if errors.Is(err, metadata.ErrUnavailable) {
		return fmt.Errorf("%s indisponible: %w", rp.Name(), err) // retried by the job runner
	}
	return fmt.Errorf("%s: %w", rp.Name(), err)
}

// pick accepts the best candidate when it is confident and clearly ahead.
func (r *Refresher) pick(cands []metadata.Candidate) (metadata.Candidate, bool) {
	if len(cands) == 0 {
		return metadata.Candidate{}, false
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	best := cands[0]
	if best.Score < r.cfg.MinScore {
		return best, false
	}
	if len(cands) > 1 && best.Score-cands[1].Score < r.cfg.MinMargin {
		return best, false
	}
	return best, true
}

// refreshEpisodes applies a season's per-episode results (and each episode's
// own local metadata) to the episode items.
func (r *Refresher) refreshEpisodes(ctx context.Context, season db.Item, sq metadata.Query, merged *metadata.Result) error {
	eps, err := r.store.ListChildItems(ctx, sql.NullInt64{Int64: season.ID, Valid: true})
	if err != nil {
		return err
	}
	for _, ep := range eps {
		q := sq
		q.Kind = metadata.KindEpisode
		q.Path = ep.Path.String
		q.Title = ep.Title
		q.IndexNumber = int(ep.IndexNumber.Int64)
		q.ParentIndex = int(season.IndexNumber.Int64)
		q.ProviderIDs = parseIDs(ep.ProviderIds)
		var layers []*metadata.Result
		for _, lp := range r.local {
			if !lp.Supports(metadata.KindEpisode) {
				continue
			}
			if res, err := lp.Local(ctx, q); err == nil && res != nil {
				layers = append(layers, res)
			}
		}
		status := ep.IdentifyStatus
		if child := merged.Children[int(ep.IndexNumber.Int64)]; ep.IndexNumber.Valid && child != nil {
			layers = append(layers, child)
			status = StatusMatched
		} else if status == StatusPending {
			status = StatusLocal
		}
		m := Merge(layers...)
		if err := r.save(ctx, ep, q.ProviderIDs, m, status); err != nil {
			return err
		}
	}
	return nil
}

// query builds the provider query for an item, including its parents'
// identifiers for seasons, episodes and albums.
func (r *Refresher) query(ctx context.Context, it db.Item) (metadata.Query, error) {
	q := metadata.Query{
		Kind: it.Kind, Title: it.Title, Year: int(it.Year.Int64), Path: it.Path.String,
		Language: *r.lang.Load(), ProviderIDs: parseIDs(it.ProviderIds),
		IndexNumber: int(it.IndexNumber.Int64), ParentIndex: int(it.ParentIndexNumber.Int64),
	}
	if !it.ParentID.Valid {
		return q, nil
	}
	parent, err := r.store.GetItem(ctx, it.ParentID.Int64)
	if err != nil {
		return q, err
	}
	switch it.Kind {
	case metadata.KindSeason, metadata.KindAlbum:
		q.ParentIDs = parseIDs(parent.ProviderIds)
		q.ParentTitle = parent.Title
	case metadata.KindEpisode:
		q.ParentIndex = int(parent.IndexNumber.Int64)
		if parent.ParentID.Valid {
			series, err := r.store.GetItem(ctx, parent.ParentID.Int64)
			if err != nil {
				return q, err
			}
			q.ParentIDs = parseIDs(series.ProviderIds)
			q.ParentTitle = series.Title
		}
	}
	return q, nil
}

// save writes merged metadata, credits and image records, then queues
// image downloads.
func (r *Refresher) save(ctx context.Context, it db.Item, ids metadata.ProviderIDs, m *metadata.Result, status string) error {
	for k, v := range m.ProviderIDs {
		if _, ok := ids[k]; !ok && v != "" {
			ids[k] = v
		}
	}
	idsJSON, _ := json.Marshal(ids)
	var toFetch []int64
	err := r.store.Tx(ctx, func(q *db.Queries) error {
		p := db.ApplyItemMetadataParams{
			OriginalTitle: nstr(m.OriginalTitle), Overview: nstr(m.Overview), Tagline: nstr(m.Tagline),
			Year: nint(int64(m.Year)), PremiereDate: nstr(m.PremiereDate), EndDate: nstr(m.EndDate),
			Status: nstr(m.Status), RuntimeMs: nint(m.RuntimeMs), OfficialRating: nstr(m.OfficialRating),
			CommunityRating: sql.NullFloat64{Float64: m.CommunityRating, Valid: m.CommunityRating > 0},
			Collection:      nstr(m.Collection), ProviderIds: string(idsJSON), IdentifyStatus: status, ID: it.ID,
		}
		if m.Title != "" {
			p.Title = nstr(m.Title)
			// Numbered kinds keep their numeric sort key from the scanner.
			if it.Kind == metadata.KindMovie || it.Kind == metadata.KindSeries || it.Kind == metadata.KindAlbum || it.Kind == metadata.KindArtist {
				st := m.SortTitle
				if st == "" {
					st = m.Title
				}
				p.SortTitle = nstr(naming.SortTitle(st))
			}
		}
		if err := q.ApplyItemMetadata(ctx, p); err != nil {
			return err
		}
		if err := store.UpdateRatingAge(ctx, q, it.ID); err != nil {
			return err
		}
		if err := saveCredits(ctx, q, it.ID, m); err != nil {
			return err
		}
		var err error
		toFetch, err = saveImages(ctx, q, it.ID, m.Images)
		return err
	})
	if err != nil {
		return err
	}
	for _, id := range toFetch {
		if _, err := r.jobs.Enqueue(ctx, JobImage, fmt.Sprint(id), imagePayload{ImageID: id}, 3); err != nil {
			return err
		}
	}
	return nil
}

func saveCredits(ctx context.Context, q *db.Queries, itemID int64, m *metadata.Result) error {
	if len(m.Genres) > 0 {
		if err := q.ClearItemGenres(ctx, itemID); err != nil {
			return err
		}
		for _, g := range m.Genres {
			if g = strings.TrimSpace(g); g == "" {
				continue
			}
			id, err := q.UpsertGenre(ctx, g)
			if err != nil {
				return err
			}
			if err := q.AddItemGenre(ctx, db.AddItemGenreParams{ItemID: itemID, GenreID: id}); err != nil {
				return err
			}
		}
	}
	if len(m.Studios) > 0 {
		if err := q.ClearItemStudios(ctx, itemID); err != nil {
			return err
		}
		for _, s := range m.Studios {
			if s = strings.TrimSpace(s); s == "" {
				continue
			}
			id, err := q.UpsertStudio(ctx, s)
			if err != nil {
				return err
			}
			if err := q.AddItemStudio(ctx, db.AddItemStudioParams{ItemID: itemID, StudioID: id}); err != nil {
				return err
			}
		}
	}
	if len(m.People) > 0 {
		if err := q.ClearItemPeople(ctx, itemID); err != nil {
			return err
		}
		for _, p := range m.People {
			name := strings.TrimSpace(p.Name)
			if name == "" {
				continue
			}
			key := "name:" + strings.ToLower(name)
			if id := p.ProviderIDs[metadata.IDTmdb]; id != "" {
				key = "tmdb:" + id
			}
			ids, _ := json.Marshal(p.ProviderIDs)
			if len(p.ProviderIDs) == 0 {
				ids = []byte("{}")
			}
			pid, err := q.UpsertPerson(ctx, db.UpsertPersonParams{Name: name, Key: key, ProviderIds: string(ids), ImageUrl: p.ImageURL})
			if err != nil {
				return err
			}
			err = q.AddItemPerson(ctx, db.AddItemPersonParams{ItemID: itemID, PersonID: pid, Kind: p.Kind, Role: p.Role, Ord: int64(p.Order)})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// saveImages upserts image records (keeping existing ones of types the new
// result does not mention) and returns ids that need fetching.
func saveImages(ctx context.Context, q *db.Queries, itemID int64, imgs []metadata.RemoteImage) ([]int64, error) {
	var toFetch []int64
	count := map[string]int64{}
	for _, img := range imgs {
		idx := count[img.Type]
		count[img.Type]++
		row, err := q.UpsertImage(ctx, db.UpsertImageParams{
			ItemID: itemID, Type: img.Type, Idx: idx, Source: img.URL,
			IsLocal: b2i(img.Local), Provider: providerOf(img), Language: img.Language,
		})
		if err != nil {
			return nil, err
		}
		if !row.Hash.Valid {
			toFetch = append(toFetch, row.ID)
		}
	}
	for typ, n := range count {
		if err := q.DeleteImagesFrom(ctx, db.DeleteImagesFromParams{ItemID: itemID, Type: typ, Idx: n}); err != nil {
			return nil, err
		}
	}
	return toFetch, nil
}

// HandleImage is the jobs.Handler for JobImage.
func (r *Refresher) HandleImage(ctx context.Context, j jobs.Job) error {
	var p imagePayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
	}
	img, err := r.store.GetImage(ctx, p.ImageID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if img.Hash.Valid {
		return nil
	}
	info, err := r.ingest(ctx, img.Source)
	if err != nil {
		if errors.Is(err, jobs.ErrPermanent) || j.Attempts >= 3 {
			_ = r.store.SetImageError(context.WithoutCancel(ctx), db.SetImageErrorParams{FetchError: err.Error(), ID: img.ID})
		}
		return err
	}
	err = r.store.SetImageFetched(ctx, db.SetImageFetchedParams{
		Hash: nstr(info.Hash), Width: int64(info.Width), Height: int64(info.Height),
		Blurhash: info.Blurhash, Dominant: info.Dominant, ID: img.ID,
	})
	if err == nil {
		r.hub.Publish("item.image", map[string]any{"id": img.ItemID, "type": img.Type})
	}
	return err
}

// Search lists identification candidates for an item (manual identify).
func (r *Refresher) Search(ctx context.Context, itemID int64, title string, year int) ([]metadata.Candidate, error) {
	it, err := r.store.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	q, err := r.query(ctx, it)
	if err != nil {
		return nil, err
	}
	if title != "" {
		q.Title = title
	}
	q.Year = year
	q.ProviderIDs = metadata.ProviderIDs{}
	var out []metadata.Candidate
	for _, rp := range r.remote {
		if !rp.Supports(it.Kind) {
			continue
		}
		if e, ok := rp.(Enabler); ok && !e.Enabled() {
			continue
		}
		c, err := rp.Search(ctx, q)
		if err != nil {
			return nil, err
		}
		out = append(out, c...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// Identify pins an item to provider ids chosen by the user and refreshes it.
func (r *Refresher) Identify(ctx context.Context, itemID int64, ids metadata.ProviderIDs) error {
	if _, err := r.store.GetItem(ctx, itemID); err != nil {
		return err
	}
	b, _ := json.Marshal(ids)
	if err := r.store.SetIdentify(ctx, db.SetIdentifyParams{ProviderIds: string(b), IdentifyStatus: StatusManual, ID: itemID}); err != nil {
		return err
	}
	_, err := r.Enqueue(ctx, itemID, false)
	return err
}

// primaryID is the provider id that identifies an item kind.
func primaryID(kind string) string {
	switch kind {
	case metadata.KindMovie, metadata.KindSeries, metadata.KindSeason, metadata.KindEpisode:
		return metadata.IDTmdb
	case metadata.KindAlbum:
		return metadata.IDMusicBrainzRelGrp
	case metadata.KindArtist:
		return metadata.IDMusicBrainzArtist
	}
	return ""
}

// identifiable kinds are searched by title; seasons and episodes are fetched
// through their series instead.
func identifiable(kind string) bool {
	switch kind {
	case metadata.KindMovie, metadata.KindSeries, metadata.KindAlbum, metadata.KindArtist:
		return true
	}
	return false
}

func providerOf(img metadata.RemoteImage) string {
	switch {
	case img.Local:
		return "local"
	case strings.Contains(img.URL, "tmdb.org"):
		return "tmdb"
	case strings.Contains(img.URL, "coverartarchive.org"):
		return "coverartarchive"
	}
	return ""
}

func parseIDs(s string) metadata.ProviderIDs {
	ids := metadata.ProviderIDs{}
	_ = json.Unmarshal([]byte(s), &ids)
	return ids
}

func nstr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func nint(v int64) sql.NullInt64   { return sql.NullInt64{Int64: v, Valid: v != 0} }
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
