package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// itemColumns lists items columns in db.Item field order (qualified as i.*).
const itemColumns = `i.id, i.library_id, i.parent_id, i.kind, i.title, i.sort_title, i.original_title,
	i.year, i.overview, i.path, i.size, i.mtime, i.runtime_ms, i.index_number, i.parent_index_number,
	i.provider_ids, i.locked_fields, i.missing, i.date_added, i.date_updated, i.key, i.edition,
	i.air_date, i.index_number_end, i.tagline, i.premiere_date, i.end_date, i.status,
	i.official_rating, i.community_rating, i.collection, i.identify_status, i.metadata_refreshed_at`

// Extra per-row facts used by list views: child count and best video stream.
const browseExtras = `,
	(SELECT count(*) FROM items c WHERE c.parent_id = i.id AND c.missing = 0),
	COALESCE((SELECT max(ms.width) FROM media_files mf JOIN media_streams ms ON ms.media_file_id = mf.id
	          WHERE mf.item_id = i.id AND ms.type = 'video'), 0),
	COALESCE((SELECT max(ms.height) FROM media_files mf JOIN media_streams ms ON ms.media_file_id = mf.id
	          WHERE mf.item_id = i.id AND ms.type = 'video'), 0),
	COALESCE((SELECT max(ms.hdr) FROM media_files mf JOIN media_streams ms ON ms.media_file_id = mf.id
	          WHERE mf.item_id = i.id AND ms.type = 'video'), ''),
	(NOT EXISTS (SELECT 1 FROM media_files mf WHERE mf.item_id = i.id)
	 OR EXISTS (SELECT 1 FROM media_files mf LEFT JOIN library_sources ls ON ls.id = mf.source_id
	            WHERE mf.item_id = i.id AND COALESCE(ls.status, 'online') != 'offline'))`

// Viewer data; the unplayed-episode count takes the user id as first argument.
const userExtras = `,
	CASE WHEN i.kind IN ('series', 'season') THEN (
		SELECT count(*) FROM items e WHERE e.kind = 'episode' AND e.missing = 0
		  AND (e.parent_id = i.id OR e.parent_id IN (SELECT se.id FROM items se WHERE se.parent_id = i.id))
		  AND NOT EXISTS (SELECT 1 FROM user_item_data x WHERE x.item_id = e.id AND x.user_id = ? AND x.played = 1))
	END,
	COALESCE(ud.position_ms, 0), COALESCE(ud.played, 0) = 1, COALESCE(ud.play_count, 0), ud.last_played_at,
	COALESCE(ud.is_favorite, 0) = 1, ud.rating`

// Video width thresholds for resolution classes (width is robust to
// letterboxing, height is a fallback for narrow/portrait encodes).
const (
	res4K    = "(vw >= 3200 OR vh >= 1800)"
	res1080p = "((vw >= 1800 OR vh >= 1000) AND NOT (vw >= 3200 OR vh >= 1800))"
	res720p  = "((vw >= 1200 OR vh >= 700) AND NOT (vw >= 1800 OR vh >= 1000))"
	resSD    = "(vw > 0 AND vw < 1200 AND vh < 700)"
)

// TopLevelKinds are listed when neither a parent nor kinds are requested.
var TopLevelKinds = []string{"movie", "series", "artist"}

// Viewer restricts listings to what a user may see and attaches their data.
// The zero Viewer sees everything and has no user data (internal use).
type Viewer struct {
	UserID       int64
	LibraryIDs   []int64 // nil = all libraries
	MaxAge       int     // parental limit, used when HasMaxAge
	HasMaxAge    bool
	BlockUnrated bool
}

// Visibility returns the SQL condition (on alias i) hiding items the viewer
// may not see.
func (v Viewer) Visibility() (string, []any) {
	var conds []string
	var args []any
	if v.LibraryIDs != nil {
		if len(v.LibraryIDs) == 0 {
			return "0", nil
		}
		conds = append(conds, "i.library_id IN ("+placeholders(len(v.LibraryIDs))+")")
		args = append(args, anys(v.LibraryIDs)...)
	}
	// An item inherits the classification of its series/season.
	const lineage = "a.id IN (i.id, i.parent_id, (SELECT p.parent_id FROM items p WHERE p.id = i.parent_id))"
	if v.HasMaxAge {
		conds = append(conds, "NOT EXISTS (SELECT 1 FROM items a WHERE "+lineage+" AND a.rating_age > ?)")
		args = append(args, v.MaxAge)
	}
	if v.BlockUnrated {
		conds = append(conds, "(i.kind IN ('artist', 'album', 'track') OR EXISTS (SELECT 1 FROM items a WHERE "+lineage+" AND a.rating_age IS NOT NULL))")
	}
	if len(conds) == 0 {
		return "1", nil
	}
	return strings.Join(conds, " AND "), args
}

// BrowseParams filters and orders an item listing. Zero values mean "any".
type BrowseParams struct {
	Viewer     Viewer
	Played     *bool
	InProgress bool
	Favorite   bool
	LibraryID  int64
	ParentID   int64
	Kinds      []string
	Genre      string
	PersonID   int64
	YearFrom   int
	YearTo     int
	Resolution string // "4K" | "1080p" | "720p" | "SD"
	HDR        bool
	Identify   string
	HasImage   string // only items with a fetched image of this type
	IDs        []int64
	Sort       string // title | dateAdded | year | rating | runtime | premiereDate | random | latest
	Desc       bool
	Seed       int64 // random order seed (stable across pages)
	Limit      int
	Offset     int
}

// BrowseRow is an item plus list-view facts.
type BrowseRow struct {
	db.Item
	ChildCount  int64
	VideoWidth  int64
	VideoHeight int64
	HDR         string
	Available   bool // false when every file sits on an offline source
	// Viewer's data (zero without a viewer).
	PositionMs    int64
	Played        bool
	PlayCount     int64
	LastPlayedAt  sql.NullInt64
	Favorite      bool
	UserRating    sql.NullFloat64
	UnplayedCount sql.NullInt64 // series and seasons
}

// Browse lists items with dynamic filters and ordering, and the total
// number of matches.
func (s *Store) Browse(ctx context.Context, p BrowseParams) ([]BrowseRow, int, error) {
	var where []string
	var args []any
	add := func(cond string, a ...any) { where = append(where, cond); args = append(args, a...) }

	add("i.missing = 0")
	if vis, vargs := p.Viewer.Visibility(); vis != "1" {
		add(vis, vargs...)
	}
	if p.Played != nil {
		if *p.Played {
			add("COALESCE(ud.played, 0) = 1")
		} else {
			add("COALESCE(ud.played, 0) = 0")
		}
	}
	if p.InProgress {
		add("COALESCE(ud.position_ms, 0) > 0")
	}
	if p.Favorite {
		add("COALESCE(ud.is_favorite, 0) = 1")
	}
	if p.LibraryID != 0 {
		add("i.library_id = ?", p.LibraryID)
	}
	if p.ParentID != 0 {
		add("i.parent_id = ?", p.ParentID)
	}
	kinds := p.Kinds
	if len(kinds) == 0 && p.ParentID == 0 && len(p.IDs) == 0 {
		kinds = TopLevelKinds
	}
	if len(kinds) > 0 {
		add("i.kind IN ("+placeholders(len(kinds))+")", anys(kinds)...)
	}
	if len(p.IDs) > 0 {
		add("i.id IN ("+placeholders(len(p.IDs))+")", anys(p.IDs)...)
	}
	if p.Genre != "" {
		add(`EXISTS (SELECT 1 FROM item_genres ig JOIN genres g ON g.id = ig.genre_id
		     WHERE ig.item_id = i.id AND g.name = ? COLLATE NOCASE)`, p.Genre)
	}
	if p.PersonID != 0 {
		add("EXISTS (SELECT 1 FROM item_people ip WHERE ip.item_id = i.id AND ip.person_id = ?)", p.PersonID)
	}
	if p.YearFrom != 0 {
		add("i.year >= ?", p.YearFrom)
	}
	if p.YearTo != 0 {
		add("i.year <= ?", p.YearTo)
	}
	if p.Identify != "" {
		add("i.identify_status = ?", p.Identify)
	}
	if p.HasImage != "" {
		add("EXISTS (SELECT 1 FROM images im WHERE im.item_id = i.id AND im.type = ? AND im.hash IS NOT NULL)", p.HasImage)
	}
	if p.Resolution != "" {
		cond, ok := map[string]string{"4K": res4K, "1080p": res1080p, "720p": res720p, "SD": resSD}[p.Resolution]
		if !ok {
			return nil, 0, fmt.Errorf("unknown resolution %q", p.Resolution)
		}
		add(`EXISTS (SELECT 1 FROM (SELECT max(ms.width) AS vw, max(ms.height) AS vh FROM media_files mf
		     JOIN media_streams ms ON ms.media_file_id = mf.id WHERE mf.item_id = i.id AND ms.type = 'video') v
		     WHERE ` + cond + ")")
	}
	if p.HDR {
		add(`EXISTS (SELECT 1 FROM media_files mf JOIN media_streams ms ON ms.media_file_id = mf.id
		     WHERE mf.item_id = i.id AND ms.type = 'video' AND ms.hdr != '')`)
	}
	cond := strings.Join(where, " AND ")

	join := " LEFT JOIN user_item_data ud ON ud.item_id = i.id AND ud.user_id = ? "
	joinArgs := []any{p.Viewer.UserID}
	var total int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM items i"+join+"WHERE "+cond, append(joinArgs, args...)...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dir := "ASC"
	if p.Desc {
		dir = "DESC"
	}
	var order string
	switch p.Sort {
	case "", "title":
		order = "i.sort_title " + dir
	case "dateAdded":
		order = "i.date_added " + dir
	case "year":
		order = "i.year IS NULL, i.year " + dir + ", i.sort_title"
	case "rating":
		order = "i.community_rating IS NULL, i.community_rating " + dir + ", i.sort_title"
	case "runtime":
		order = "i.runtime_ms IS NULL, i.runtime_ms " + dir + ", i.sort_title"
	case "premiereDate":
		order = "i.premiere_date IS NULL, i.premiere_date " + dir + ", i.sort_title"
	case "random":
		// Deterministic shuffle so pages do not overlap.
		order = fmt.Sprintf("((i.id * 2654435761 + %d) %% 4294967291)", p.Seed%4294967291)
	case "lastPlayed":
		order = "ud.last_played_at IS NULL, ud.last_played_at " + dir
	case "latest":
		// Series by their most recently added episode.
		order = `COALESCE((SELECT max(e.date_added) FROM items se JOIN items e ON e.parent_id = se.id
		          WHERE se.parent_id = i.id AND e.missing = 0), i.date_added) ` + dir
	default:
		return nil, 0, fmt.Errorf("unknown sort %q", p.Sort)
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT " + itemColumns + browseExtras + userExtras + " FROM items i" + join + "WHERE " + cond +
		" ORDER BY " + order + ", i.id LIMIT ? OFFSET ?"
	all := append([]any{p.Viewer.UserID}, joinArgs...)
	all = append(all, args...)
	rows, err := s.DB.QueryContext(ctx, q, append(all, limit, p.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []BrowseRow
	for rows.Next() {
		var r BrowseRow
		if err := rows.Scan(itemDest(&r.Item, &r.ChildCount, &r.VideoWidth, &r.VideoHeight, &r.HDR, &r.Available,
			&r.UnplayedCount, &r.PositionMs, &r.Played, &r.PlayCount, &r.LastPlayedAt, &r.Favorite, &r.UserRating)...); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func itemDest(i *db.Item, extra ...any) []any {
	return append([]any{&i.ID, &i.LibraryID, &i.ParentID, &i.Kind, &i.Title, &i.SortTitle, &i.OriginalTitle,
		&i.Year, &i.Overview, &i.Path, &i.Size, &i.Mtime, &i.RuntimeMs, &i.IndexNumber, &i.ParentIndexNumber,
		&i.ProviderIds, &i.LockedFields, &i.Missing, &i.DateAdded, &i.DateUpdated, &i.Key, &i.Edition,
		&i.AirDate, &i.IndexNumberEnd, &i.Tagline, &i.PremiereDate, &i.EndDate, &i.Status,
		&i.OfficialRating, &i.CommunityRating, &i.Collection, &i.IdentifyStatus, &i.MetadataRefreshedAt}, extra...)
}

// Resolution classifies a video size like the browse filter does.
func Resolution(w, h int64) string {
	switch {
	case w == 0 && h == 0:
		return ""
	case w >= 3200 || h >= 1800:
		return "4K"
	case w >= 1800 || h >= 1000:
		return "1080p"
	case w >= 1200 || h >= 700:
		return "720p"
	}
	return "SD"
}

// LibraryFilters are the facets available in a library.
type LibraryFilters struct {
	Genres      []GenreCount
	YearMin     int
	YearMax     int
	Resolutions []string
	HasHDR      bool
}

type GenreCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Filters computes facets over a library's top-level items.
func (s *Store) Filters(ctx context.Context, libID int64) (LibraryFilters, error) {
	var f LibraryFilters
	kinds := "('movie', 'series', 'artist', 'album')"
	rows, err := s.DB.QueryContext(ctx, `SELECT g.name, count(*) FROM genres g
		JOIN item_genres ig ON ig.genre_id = g.id JOIN items i ON i.id = ig.item_id
		WHERE i.library_id = ? AND i.missing = 0 AND i.kind IN `+kinds+`
		GROUP BY g.id ORDER BY count(*) DESC, g.name`, libID)
	if err != nil {
		return f, err
	}
	for rows.Next() {
		var g GenreCount
		if err := rows.Scan(&g.Name, &g.Count); err != nil {
			rows.Close()
			return f, err
		}
		f.Genres = append(f.Genres, g)
	}
	rows.Close()
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(min(year), 0), COALESCE(max(year), 0) FROM items
		WHERE library_id = ? AND missing = 0 AND year > 0 AND kind IN `+kinds, libID).Scan(&f.YearMin, &f.YearMax); err != nil {
		return f, err
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT max(ms.width), max(ms.height), max(ms.hdr) FROM media_files mf
		JOIN media_streams ms ON ms.media_file_id = mf.id
		WHERE mf.library_id = ? AND ms.type = 'video' GROUP BY mf.item_id`, libID)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var w, h int64
		var hdr string
		if err := rows.Scan(&w, &h, &hdr); err != nil {
			return f, err
		}
		if r := Resolution(w, h); r != "" {
			seen[r] = true
		}
		f.HasHDR = f.HasHDR || hdr != ""
	}
	for _, r := range []string{"4K", "1080p", "720p", "SD"} {
		if seen[r] {
			f.Resolutions = append(f.Resolutions, r)
		}
	}
	return f, rows.Err()
}

// SearchItems runs a prefix full-text search over titles; results are ranked.
func (s *Store) SearchItems(ctx context.Context, query string, limit int) ([]db.Item, error) {
	match := FTSQuery(query)
	if match == "" {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT "+itemColumns+` FROM items_fts f JOIN items i ON i.id = f.rowid
		WHERE items_fts MATCH ? AND i.missing = 0 ORDER BY bm25(items_fts, 10.0, 5.0), i.sort_title LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.Item
	for rows.Next() {
		var it db.Item
		if err := rows.Scan(itemDest(&it)...); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// PersonHit is a person matching a search, with their number of credits.
type PersonHit struct {
	ID       int64
	Name     string
	ImageURL string
	Count    int
}

// SearchPeople runs a prefix full-text search over people names.
func (s *Store) SearchPeople(ctx context.Context, query string, limit int) ([]PersonHit, error) {
	match := FTSQuery(query)
	if match == "" {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.id, p.name, p.image_url,
		(SELECT count(*) FROM item_people ip JOIN items i ON i.id = ip.item_id WHERE ip.person_id = p.id AND i.missing = 0) AS n
		FROM people_fts f JOIN people p ON p.id = f.rowid
		WHERE people_fts MATCH ? ORDER BY n = 0, bm25(people_fts), n DESC LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PersonHit
	for rows.Next() {
		var h PersonHit
		if err := rows.Scan(&h.ID, &h.Name, &h.ImageURL, &h.Count); err != nil {
			return nil, err
		}
		if h.Count > 0 {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}

// FTSQuery turns user input into a safe FTS5 query: every word becomes a
// quoted prefix term, all terms required. Returns "" when nothing is left.
func FTSQuery(q string) string {
	words := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	terms := make([]string, 0, len(words))
	for _, w := range words {
		terms = append(terms, `"`+w+`"*`)
	}
	return strings.Join(terms, " ")
}

// SimilarIDs returns items of the same kind sharing the most genres.
func (s *Store) SimilarIDs(ctx context.Context, itemID int64, kind string, limit int) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id FROM item_genres ig
		JOIN item_genres ig2 ON ig2.genre_id = ig.genre_id
		JOIN items i ON i.id = ig2.item_id
		WHERE ig.item_id = ? AND i.id != ? AND i.kind = ? AND i.missing = 0
		GROUP BY i.id ORDER BY count(*) DESC, i.community_rating DESC, i.sort_title LIMIT ?`, itemID, itemID, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// TopGenres returns the genres with the most top-level items.
func (s *Store) TopGenres(ctx context.Context, limit int) ([]GenreCount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT g.name, count(*) FROM genres g
		JOIN item_genres ig ON ig.genre_id = g.id JOIN items i ON i.id = ig.item_id
		WHERE i.missing = 0 AND i.kind IN ('movie', 'series')
		GROUP BY g.id ORDER BY count(*) DESC, g.name LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GenreCount
	for rows.Next() {
		var g GenreCount
		if err := rows.Scan(&g.Name, &g.Count); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GenreImage picks an image (backdrop preferred) from an item of a genre.
func (s *Store) GenreImage(ctx context.Context, genre string, seed int64) (db.Image, bool, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT im.id, im.item_id, im.type, im.idx, im.source, im.is_local, im.provider,
		im.language, im.hash, im.width, im.height, im.blurhash, im.dominant, im.fetch_error
		FROM images im JOIN item_genres ig ON ig.item_id = im.item_id JOIN genres g ON g.id = ig.genre_id
		JOIN items i ON i.id = im.item_id
		WHERE g.name = ? COLLATE NOCASE AND im.hash IS NOT NULL AND im.idx = 0 AND i.missing = 0
		  AND im.type IN ('backdrop', 'poster')
		ORDER BY im.type = 'backdrop' DESC, ((im.item_id * 2654435761 + ?) % 4294967291) LIMIT 1`, genre, seed%4294967291)
	var im db.Image
	err := row.Scan(&im.ID, &im.ItemID, &im.Type, &im.Idx, &im.Source, &im.IsLocal, &im.Provider,
		&im.Language, &im.Hash, &im.Width, &im.Height, &im.Blurhash, &im.Dominant, &im.FetchError)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return im, false, nil
		}
		return im, false, err
	}
	return im, true, nil
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?, ", n), ", ") }

func anys[T any](v []T) []any {
	out := make([]any, len(v))
	for i := range v {
		out[i] = v[i]
	}
	return out
}

// ImagesFor loads fetched images for many items at once.
func (s *Store) ImagesFor(ctx context.Context, ids []int64) (map[int64][]db.Image, error) {
	out := make(map[int64][]db.Image, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, item_id, type, idx, source, is_local, provider, language, hash,
		width, height, blurhash, dominant, fetch_error FROM images
		WHERE hash IS NOT NULL AND item_id IN (`+placeholders(len(ids))+`) ORDER BY item_id, type, idx`, anys(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var im db.Image
		if err := rows.Scan(&im.ID, &im.ItemID, &im.Type, &im.Idx, &im.Source, &im.IsLocal, &im.Provider,
			&im.Language, &im.Hash, &im.Width, &im.Height, &im.Blurhash, &im.Dominant, &im.FetchError); err != nil {
			return nil, err
		}
		out[im.ItemID] = append(out[im.ItemID], im)
	}
	return out, rows.Err()
}

// NextUpIDs returns, for each series the user has started (most recent
// first), the next unwatched episode after the last one watched. Specials
// (season 0) are skipped; episodes already in progress are left to
// "continue watching".
func (s *Store) NextUpIDs(ctx context.Context, userID int64, limit int) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
WITH watched AS (
    SELECT se.parent_id AS series_id, e.id AS ep_id, se.index_number AS season, e.sort_title AS st, ud.last_played_at AS lp,
           row_number() OVER (PARTITION BY se.parent_id ORDER BY ud.last_played_at DESC, se.index_number DESC, e.sort_title DESC) AS rn
    FROM user_item_data ud
    JOIN items e ON e.id = ud.item_id AND e.kind = 'episode' AND e.missing = 0
    JOIN items se ON se.id = e.parent_id
    WHERE ud.user_id = ?1 AND ud.played = 1
)
SELECT (SELECT n.id FROM items n JOIN items ns ON ns.id = n.parent_id
        WHERE ns.parent_id = w.series_id AND n.kind = 'episode' AND n.missing = 0 AND ns.index_number > 0
          AND (ns.index_number > w.season OR (ns.index_number = w.season AND n.sort_title > w.st))
          AND NOT EXISTS (SELECT 1 FROM user_item_data x WHERE x.user_id = ?1 AND x.item_id = n.id
                          AND (x.played = 1 OR x.position_ms > 0))
        ORDER BY ns.index_number, n.sort_title LIMIT 1) AS next_id
FROM watched w WHERE w.rn = 1 ORDER BY w.lp DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() && len(ids) < limit {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id.Valid {
			ids = append(ids, id.Int64)
		}
	}
	return ids, rows.Err()
}
