package nfo

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

// ErrEmpty is returned by ParseNFO when the content holds no usable metadata.
var ErrEmpty = errors.New("nfo: no metadata")

// ParseNFO parses a Kodi-style NFO (as written by Kodi, Jellyfin, Emby,
// tinyMediaManager...) for an item of the given kind. URL-only NFOs and
// URLs written after the XML yield provider ids. For episodes, a
// multi-episode NFO (several <episodedetails>) gives the first episode in
// the Result and the others in Children, keyed by episode number. For
// albums, <track> elements go to Children keyed by position.
func ParseNFO(r io.Reader, kind string) (*metadata.Result, error) {
	doc, err := decode(r)
	if err != nil {
		return nil, err
	}
	var roots []*node
	if kind == metadata.KindEpisode {
		for _, n := range doc.roots {
			if n.name == "episodedetails" {
				roots = append(roots, n)
			}
		}
	}
	if len(roots) == 0 && len(doc.roots) > 0 {
		roots = doc.roots[:1]
	}

	var res *metadata.Result
	for i, root := range roots {
		r := mapNode(root, kind)
		if i == 0 {
			res = r
			continue
		}
		key := r.IndexNumber
		if key == 0 {
			key = i + 1
		}
		if res.Children == nil {
			res.Children = map[int]*metadata.Result{}
		}
		if _, dup := res.Children[key]; !dup && key != res.IndexNumber {
			res.Children[key] = r
		}
	}
	if res == nil {
		res = &metadata.Result{}
	}
	if res.ProviderIDs == nil {
		res.ProviderIDs = metadata.ProviderIDs{}
	}
	idsFromText(res.ProviderIDs, doc.outside)
	if len(res.ProviderIDs) == 0 {
		res.ProviderIDs = nil
	}
	if isEmpty(res) {
		return nil, ErrEmpty
	}
	return res, nil
}

func isEmpty(r *metadata.Result) bool {
	return r.Title == "" && r.OriginalTitle == "" && r.Overview == "" && r.Year == 0 &&
		r.PremiereDate == "" && len(r.ProviderIDs) == 0 && len(r.Genres) == 0 &&
		len(r.People) == 0 && len(r.Children) == 0 && r.IndexNumber == 0 && len(r.Images) == 0 &&
		r.Collection == ""
}

var (
	leadingIntRe = regexp.MustCompile(`^\s*(\d+)`)
	dateRe       = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})`)
	yearOnlyRe   = regexp.MustCompile(`^(\d{4})$`)
	durationRe   = regexp.MustCompile(`^(?:(\d+):)?(\d+):(\d{1,2})$`)
	ratedRe      = regexp.MustCompile(`(?i)^rated\s+`)
	countryRe    = regexp.MustCompile(`^([A-Za-z]{2,}(?: [A-Za-z]+)?)\s*:\s*(.+)$`)
	tmdbAPIRe    = regexp.MustCompile(`(?i)api\.themoviedb\.org/\d+/tv/(\d+)`)
	tvdbAPIRe    = regexp.MustCompile(`(?i)thetvdb\.com/api/[^/]+/series/(\d+)`)
)

// mapNode converts one NFO root element to a Result.
func mapNode(n *node, kind string) *metadata.Result {
	r := &metadata.Result{ProviderIDs: metadata.ProviderIDs{}}
	r.Title = n.childText("title", "localtitle", "name")
	r.OriginalTitle = n.childText("originaltitle")
	r.SortTitle = n.childText("sorttitle", "sortname")
	r.Overview = n.childText("plot", "biography", "review", "outline", "albumdesc")
	r.Tagline = n.childText("tagline")
	r.Status = normStatus(n.childText("status"))

	r.PremiereDate = normDate(n.childText("premiered", "aired", "releasedate", "formed", "born"))
	r.EndDate = normDate(n.childText("enddate", "disbanded", "died"))
	r.Year = atoi(n.childText("year"))
	if r.Year < 1000 || r.Year > 9999 {
		r.Year = 0
	}
	if r.Year == 0 && len(r.PremiereDate) >= 4 {
		r.Year = atoi(r.PremiereDate[:4])
	}
	if r.PremiereDate == "" {
		// Kodi music NFOs often only carry a year in <releasedate>.
		if m := yearOnlyRe.FindStringSubmatch(n.childText("releasedate", "formed", "born")); m != nil && r.Year == 0 {
			r.Year = atoi(m[1])
		}
	}

	if m := leadingIntRe.FindStringSubmatch(n.childText("runtime")); m != nil {
		r.RuntimeMs = int64(atoi(m[1])) * 60_000
	}
	if r.RuntimeMs == 0 {
		if fi := n.child("fileinfo"); fi != nil {
			if sd := fi.child("streamdetails"); sd != nil {
				if v := sd.child("video"); v != nil {
					r.RuntimeMs = int64(atoi(v.childText("durationinseconds"))) * 1000
				}
			}
		}
	}
	r.OfficialRating = normCertification(n.childText("mpaa", "certification", "contentrating"))
	r.CommunityRating = communityRating(n)

	r.Genres = splitList(n, "genre")
	studios := splitList(n, "studio")
	if kind == metadata.KindAlbum {
		studios = append(studios, splitList(n, "label")...)
	}
	r.Studios = studios
	r.People = people(n, kind)

	collectionAndIDs(n, r, kind)

	switch kind {
	case metadata.KindEpisode:
		r.ParentIndex = atoi(n.childText("season"))
		r.IndexNumber = atoi(n.childText("episode"))
	case metadata.KindSeason:
		r.IndexNumber = atoi(n.childText("seasonnumber"))
	case metadata.KindSeries:
		for _, ns := range n.all("namedseason") {
			num, err := strconv.Atoi(ns.Attr("number"))
			if err != nil || ns.Text() == "" {
				continue
			}
			if r.Children == nil {
				r.Children = map[int]*metadata.Result{}
			}
			r.Children[num] = &metadata.Result{Title: ns.Text(), IndexNumber: num}
		}
	case metadata.KindAlbum:
		albumTracks(n, r)
	}
	r.Images = nfoImages(n, kind)
	return r
}

func atoi(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func normDate(s string) string {
	m := dateRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return ""
	}
	mo, d := atoi(m[2]), atoi(m[3])
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return ""
	}
	return m[1] + "-" + pad2(mo) + "-" + pad2(d)
}

func pad2(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

func normStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return ""
	case "continuing", "returning series", "returning", "in production", "airing", "planned", "pilot":
		return "continuing"
	case "ended", "canceled", "cancelled", "finished":
		return "ended"
	}
	return ""
}

// normCertification turns "Rated PG-13", "FR:12 / US:PG-13", "France:12"
// into a bare rating, preferring the French then the US one.
func normCertification(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '|' })
	byCountry := map[string]string{}
	var first string
	for _, p := range parts {
		p = strings.TrimSpace(ratedRe.ReplaceAllString(strings.TrimSpace(p), ""))
		if p == "" {
			continue
		}
		country := ""
		if m := countryRe.FindStringSubmatch(p); m != nil {
			country, p = strings.ToLower(m[1]), strings.TrimSpace(ratedRe.ReplaceAllString(m[2], ""))
		}
		switch country {
		case "france":
			country = "fr"
		case "usa", "united states", "us":
			country = "us"
		}
		if first == "" {
			first = p
		}
		if _, ok := byCountry[country]; !ok {
			byCountry[country] = p
		}
	}
	for _, c := range []string{"fr", "us", ""} {
		if v := byCountry[c]; v != "" {
			return v
		}
	}
	return first
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
	if err != nil {
		return 0
	}
	return v
}

// communityRating reads <ratings><rating default="true" max="10"><value>,
// falling back to the first rating, then to a plain <rating>; normalised
// to 0-10.
func communityRating(n *node) float64 {
	scale := func(v float64, maxAttr string) float64 {
		if m := parseFloat(maxAttr); m > 0 && m != 10 {
			v = v / m * 10
		}
		if v < 0 || v > 10 {
			return 0
		}
		return v
	}
	if rs := n.child("ratings"); rs != nil {
		var best *node
		for _, c := range rs.all("rating") {
			if best == nil || strings.EqualFold(c.Attr("default"), "true") {
				best = c
				if strings.EqualFold(c.Attr("default"), "true") {
					break
				}
			}
		}
		if best != nil {
			val := best.childText("value")
			if val == "" {
				val = best.Text()
			}
			if v := scale(parseFloat(val), best.Attr("max")); v > 0 {
				return v
			}
		}
	}
	for _, c := range n.all("rating") {
		if v := scale(parseFloat(c.Text()), c.Attr("max")); v > 0 {
			return v
		}
	}
	return 0
}

// splitList returns the values of every <name> child, splitting "A / B" and
// "A | B", de-duplicated case-insensitively.
func splitList(n *node, name string) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range n.all(name) {
		for _, p := range strings.FieldsFunc(c.Text(), func(r rune) bool { return r == '/' || r == '|' }) {
			p = strings.TrimSpace(p)
			if k := strings.ToLower(p); p != "" && !seen[k] {
				seen[k] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func people(n *node, kind string) []metadata.Person {
	var out []metadata.Person
	seen := map[string]bool{}
	add := func(p metadata.Person) {
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			return
		}
		k := p.Kind + "\x00" + strings.ToLower(p.Name) + "\x00" + p.Role
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, p)
	}
	crew := func(tag, pkind string) {
		for _, c := range n.all(tag) {
			name := c.childText("name")
			if name == "" {
				name = c.Text()
			}
			role := c.childText("role")
			for _, part := range strings.Split(name, " / ") {
				add(metadata.Person{Name: part, Kind: pkind, Role: role, ProviderIDs: personIDs(c)})
			}
		}
	}

	var actors []metadata.Person
	for i, c := range n.all("actor") {
		name := c.childText("name")
		if name == "" && len(c.children) == 0 {
			name = c.Text()
		}
		p := metadata.Person{
			Name:        name,
			Role:        c.childText("role"),
			Kind:        "actor",
			Order:       i,
			ImageURL:    usableURL(c.childText("thumb")),
			ProviderIDs: personIDs(c),
		}
		if o := c.childText("order", "sortorder"); o != "" {
			p.Order = atoi(o)
		}
		switch strings.ToLower(strings.ReplaceAll(c.childText("type"), " ", "")) {
		case "gueststar", "guest":
			p.Kind = "guest"
		case "director":
			p.Kind = "director"
		case "writer":
			p.Kind = "writer"
		case "producer":
			p.Kind = "producer"
		case "composer":
			p.Kind = "composer"
		}
		actors = append(actors, p)
	}
	sort.SliceStable(actors, func(i, j int) bool { return actors[i].Order < actors[j].Order })
	for _, p := range actors {
		add(p)
	}
	crew("director", "director")
	crew("credits", "writer")
	crew("writer", "writer")
	crew("producer", "producer")
	crew("composer", "composer")
	if kind == metadata.KindAlbum {
		albumArtists(n, add)
	}
	return out
}

// albumArtists adds the album artists as Person{Kind: "artist"}. The
// contract has no album-artist field; see the package documentation.
func albumArtists(n *node, add func(metadata.Person)) {
	credits := n.all("albumartistcredits")
	for i, c := range credits {
		name := c.childText("artist")
		ids := metadata.ProviderIDs{}
		setID(ids, metadata.IDMusicBrainzArtist, strings.ToLower(c.childText("musicbrainzartistid")))
		if len(ids) == 0 {
			ids = nil
		}
		add(metadata.Person{Name: name, Kind: "artist", Order: i, ProviderIDs: ids})
	}
	if len(credits) > 0 {
		return
	}
	mbids := strings.Split(n.childText("musicbrainzalbumartistid", "musicbrainzartistid"), "/")
	for i, a := range n.all("artist") {
		name := a.childText("name")
		if name == "" {
			name = a.Text()
		}
		var ids metadata.ProviderIDs
		if i < len(mbids) && strings.TrimSpace(mbids[i]) != "" {
			ids = metadata.ProviderIDs{metadata.IDMusicBrainzArtist: strings.ToLower(strings.TrimSpace(mbids[i]))}
		}
		add(metadata.Person{Name: name, Kind: "artist", Order: i, ProviderIDs: ids})
	}
}

func personIDs(c *node) metadata.ProviderIDs {
	ids := metadata.ProviderIDs{}
	setID(ids, metadata.IDTmdb, c.childText("tmdbid"))
	setID(ids, metadata.IDImdb, c.childText("imdbid"))
	setID(ids, metadata.IDTvdb, c.childText("tvdbid"))
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func usableURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "/") {
		return s
	}
	return ""
}

func collectionAndIDs(n *node, r *metadata.Result, kind string) {
	ids := r.ProviderIDs
	for _, u := range n.all("uniqueid") {
		typ := strings.ToLower(u.Attr("type"))
		switch typ {
		case "tmdb", "themoviedb":
			setID(ids, metadata.IDTmdb, u.Text())
		case "imdb":
			setID(ids, metadata.IDImdb, u.Text())
		case "tvdb", "thetvdb":
			setID(ids, metadata.IDTvdb, u.Text())
		case "tmdbcollection", "tmdbset":
			setID(ids, metadata.IDTmdbCollection, u.Text())
		case "":
			legacyID(ids, u.Text(), kind)
		}
	}
	setID(ids, metadata.IDImdb, n.childText("imdbid", "imdb_id", "imdb"))
	setID(ids, metadata.IDTmdb, n.childText("tmdbid", "tmdb"))
	setID(ids, metadata.IDTvdb, n.childText("tvdbid", "tvdb"))
	setID(ids, metadata.IDTmdbCollection, n.childText("tmdbcollectionid", "collectionnumber"))
	if id := n.childText("id"); id != "" {
		legacyID(ids, id, kind)
	}

	switch kind {
	case metadata.KindAlbum:
		setID(ids, metadata.IDMusicBrainzRelease, strings.ToLower(n.childText("musicbrainzalbumid", "musicbrainzreleaseid")))
		setID(ids, metadata.IDMusicBrainzRelGrp, strings.ToLower(n.childText("musicbrainzreleasegroupid")))
	case metadata.KindArtist:
		setID(ids, metadata.IDMusicBrainzArtist, strings.ToLower(n.childText("musicbrainzartistid")))
	}

	if eg := n.child("episodeguide"); eg != nil {
		episodeGuideIDs(ids, eg)
	}

	if s := n.child("set"); s != nil {
		name := s.childText("name")
		if name == "" && s.child("name") == nil {
			name = s.Text()
		}
		r.Collection = name
		setID(ids, metadata.IDTmdbCollection, s.Attr("tmdbcolid"))
	}
	if len(ids) == 0 {
		r.ProviderIDs = nil
	}
}

// legacyID interprets a bare <id>: "tt..." is IMDb, a number is TMDb for
// movies and TheTVDB for TV.
func legacyID(ids metadata.ProviderIDs, v, kind string) {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(strings.ToLower(v), "tt") {
		setID(ids, metadata.IDImdb, v)
		return
	}
	switch kind {
	case metadata.KindMovie:
		setID(ids, metadata.IDTmdb, v)
	case metadata.KindSeries, metadata.KindSeason, metadata.KindEpisode:
		setID(ids, metadata.IDTvdb, v)
	}
}

// episodeGuideIDs reads Kodi 20+ JSON ({"tmdb":"1396","tvdb":"81189"}) or
// older <url> scraper links.
func episodeGuideIDs(ids metadata.ProviderIDs, eg *node) {
	text := eg.Text()
	for _, u := range eg.all("url") {
		text += "\n" + u.Text()
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "{") {
		var m map[string]any
		if json.Unmarshal([]byte(text), &m) == nil {
			for k, key := range map[string]string{"tmdb": metadata.IDTmdb, "tvdb": metadata.IDTvdb, "imdb": metadata.IDImdb} {
				switch v := m[k].(type) {
				case string:
					setID(ids, key, v)
				case float64:
					setID(ids, key, strconv.FormatInt(int64(v), 10))
				}
			}
			return
		}
	}
	idsFromText(ids, text)
	// "https://api.themoviedb.org/3/tv/1396?api_key=..."
	if m := tmdbAPIRe.FindStringSubmatch(text); m != nil {
		setID(ids, metadata.IDTmdb, m[1])
	}
	// "http://thetvdb.com/api/1D62F2F90030C444/series/81189/all/en.zip"
	if m := tvdbAPIRe.FindStringSubmatch(text); m != nil {
		setID(ids, metadata.IDTvdb, m[1])
	}
}

// albumTracks maps <track> elements to Children keyed by position.
func albumTracks(n *node, r *metadata.Result) {
	for i, t := range n.all("track") {
		pos := atoi(t.childText("position"))
		if pos == 0 {
			pos = i + 1
		}
		c := &metadata.Result{Title: t.childText("title"), IndexNumber: pos}
		c.ParentIndex = atoi(t.childText("disc", "discnumber"))
		if m := durationRe.FindStringSubmatch(t.childText("duration")); m != nil {
			c.RuntimeMs = int64(atoi(m[1])*3600+atoi(m[2])*60+atoi(m[3])) * 1000
		} else if secs := atoi(t.childText("duration")); secs > 0 {
			c.RuntimeMs = int64(secs) * 1000
		}
		if id := strings.ToLower(t.childText("musicbrainztrackid", "musicbrainzrecordingid")); id != "" {
			c.ProviderIDs = metadata.ProviderIDs{}
			setID(c.ProviderIDs, metadata.IDMusicBrainzTrack, id)
		}
		if c.Title == "" {
			continue
		}
		if r.Children == nil {
			r.Children = map[int]*metadata.Result{}
		}
		if _, dup := r.Children[pos]; !dup {
			r.Children[pos] = c
		}
	}
}

// nfoImages returns the remote artwork URLs listed in the NFO (<thumb
// aspect="poster">, <fanart><thumb>), Local = false.
func nfoImages(n *node, kind string) []metadata.RemoteImage {
	var out []metadata.RemoteImage
	seen := map[string]bool{}
	add := func(typ, u string) {
		u = strings.TrimSpace(u)
		if typ == "" || seen[u] || (!strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://")) {
			return
		}
		seen[u] = true
		out = append(out, metadata.RemoteImage{Type: typ, URL: u})
	}
	defType := metadata.ImagePoster
	if kind == metadata.KindEpisode {
		defType = metadata.ImageThumb
	}
	for _, t := range n.all("thumb") {
		if t.Attr("season") != "" || strings.EqualFold(t.Attr("type"), "season") {
			continue // season artwork listed in tvshow.nfo
		}
		typ := defType
		switch strings.ToLower(t.Attr("aspect")) {
		case "", "poster", "thumb":
			if strings.EqualFold(t.Attr("aspect"), "thumb") && kind != metadata.KindAlbum && kind != metadata.KindArtist {
				typ = metadata.ImageThumb
			}
		case "landscape":
			typ = metadata.ImageThumb
		case "banner":
			typ = metadata.ImageBanner
		case "clearlogo", "logo":
			typ = metadata.ImageLogo
		case "fanart":
			typ = metadata.ImageBackdrop
		default: // clearart, discart, keyart, characterart...
			typ = ""
		}
		add(typ, t.Text())
	}
	if f := n.child("fanart"); f != nil {
		base := f.Attr("url")
		for _, t := range f.all("thumb") {
			u := t.Text()
			if base != "" && !strings.HasPrefix(u, "http") {
				u = base + u
			}
			add(metadata.ImageBackdrop, u)
		}
	}
	return out
}
