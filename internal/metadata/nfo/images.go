package nfo

import (
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for DecodeConfig
	_ "image/png"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/naming"
)

var (
	imageExts = []string{"jpg", "jpeg", "png", "webp"}
	logoExts  = []string{"png", "webp", "jpg", "jpeg"}

	numberedFanartRe = regexp.MustCompile(`^(?:fanart|backdrop|background)-?(\d{1,3})\.(?:jpe?g|png|webp)$`)
	albumArtRe       = regexp.MustCompile(`^albumart.*\.jpe?g$`)
)

// collector accumulates images in preference order, without duplicates.
type collector struct {
	seen map[string]bool
	out  []metadata.RemoteImage
}

func (c *collector) add(typ, path string) {
	if path == "" || c.seen[path] {
		return
	}
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	c.seen[path] = true
	img := metadata.RemoteImage{Type: typ, URL: path, Local: true}
	img.Width, img.Height = imageSize(path)
	c.out = append(c.out, img)
}

// stems adds every "<stem>.<ext>" found in d, stems first, then extensions.
func (c *collector) stems(typ string, d *dirIndex, stems ...string) {
	if d == nil {
		return
	}
	exts := imageExts
	if typ == metadata.ImageLogo {
		exts = logoExts
	}
	for _, s := range stems {
		if s == "" {
			continue
		}
		for _, e := range exts {
			c.add(typ, d.file(s+"."+e))
		}
	}
}

// numbered adds fanart1.jpg, backdrop2.png... ordered by number.
func (c *collector) numbered(d *dirIndex) {
	if d == nil {
		return
	}
	type nf struct {
		n    int
		name string
	}
	var list []nf
	for _, l := range d.lower {
		if m := numberedFanartRe.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			list = append(list, nf{n, l})
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].n < list[j].n })
	for _, f := range list {
		c.add(metadata.ImageBackdrop, d.file(f.name))
	}
}

// extraFanart adds the images of an "extrafanart" sub-folder.
func (c *collector) extraFanart(d *dirIndex) {
	sub := d.sub("extrafanart")
	if sub == nil {
		return
	}
	for _, l := range sub.lower {
		if hasImageExt(l) {
			c.add(metadata.ImageBackdrop, sub.file(l))
		}
	}
}

func hasImageExt(name string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	for _, e := range imageExts {
		if e == ext {
			return true
		}
	}
	return false
}

// imageSize reads the dimensions of a JPEG or PNG; 0, 0 otherwise.
func imageSize(path string) (int, int) {
	f, err := files.Open(path)
	if err != nil {
		return 0, 0
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func suffixed(bases []string, suffix string) []string {
	out := make([]string, 0, len(bases))
	for _, b := range bases {
		out = append(out, b+suffix)
	}
	return out
}

func isDir(path string) bool {
	d, err := files.IsDir(path)
	return err == nil && d
}

// FindImages returns the local artwork of an item (Kodi / Jellyfin / Plex
// naming), best first within each type. URL is the absolute file path and
// Local is true. Width/Height are filled for JPEG and PNG files.
func FindImages(q metadata.Query) []metadata.RemoteImage {
	if q.Path == "" {
		return nil
	}
	var c collector
	switch q.Kind {
	case metadata.KindMovie:
		movieImages(&c, q)
	case metadata.KindSeries:
		if !isDir(q.Path) {
			return nil
		}
		d := readDir(q.Path)
		c.stems(metadata.ImagePoster, d, "poster", "folder", "cover", "default", "show")
		c.stems(metadata.ImageBackdrop, d, "fanart", "backdrop", "background")
		c.numbered(d)
		c.extraFanart(d)
		c.stems(metadata.ImageLogo, d, "logo", "clearlogo")
		c.stems(metadata.ImageBanner, d, "banner")
		c.stems(metadata.ImageThumb, d, "landscape", "thumb")
	case metadata.KindSeason:
		seasonImages(&c, q)
	case metadata.KindEpisode:
		if isDir(q.Path) {
			return nil
		}
		d := readDir(filepath.Dir(q.Path))
		b := stemOf(q.Path)
		c.stems(metadata.ImageThumb, d, b+"-thumb", b+"-landscape", b)
		c.stems(metadata.ImageThumb, d.sub("metadata"), b) // Jellyfin/Emby
	case metadata.KindAlbum:
		if !isDir(q.Path) {
			return nil
		}
		d := readDir(q.Path)
		c.stems(metadata.ImagePoster, d, "cover", "folder", "front", "album")
		var art []string
		for _, l := range d.lower {
			if albumArtRe.MatchString(l) {
				art = append(art, l)
			}
		}
		// AlbumArt_{GUID}_Large.jpg before ..._Small.jpg (Windows Media Player).
		sort.SliceStable(art, func(i, j int) bool { return artRank(art[i]) < artRank(art[j]) })
		for _, l := range art {
			c.add(metadata.ImagePoster, d.file(l))
		}
		c.stems(metadata.ImageBackdrop, d, "fanart", "backdrop")
		c.stems(metadata.ImageLogo, d, "logo", "clearlogo")
	case metadata.KindArtist:
		if !isDir(q.Path) {
			return nil
		}
		d := readDir(q.Path)
		c.stems(metadata.ImagePoster, d, "artist", "folder", "poster")
		c.stems(metadata.ImageBackdrop, d, "fanart", "backdrop", "background")
		c.numbered(d)
		c.extraFanart(d)
		c.stems(metadata.ImageLogo, d, "logo", "clearlogo")
		c.stems(metadata.ImageBanner, d, "banner")
		c.stems(metadata.ImageThumb, d, "landscape", "thumb")
	}
	return c.out
}

func artRank(name string) int {
	switch {
	case strings.Contains(name, "large"):
		return 0
	case strings.Contains(name, "small"):
		return 2
	}
	return 1
}

func movieImages(c *collector, q metadata.Query) {
	md := resolveMovie(q.Path, q.Title, q.Year)
	if md == nil {
		return
	}
	d, b := md.idx, md.bases
	if md.flat {
		// Shared folder: only artwork named after this movie.
		c.stems(metadata.ImagePoster, d, append(suffixed(b, "-poster"), b...)...)
		c.stems(metadata.ImageBackdrop, d, append(suffixed(b, "-fanart"), suffixed(b, "-backdrop")...)...)
		c.stems(metadata.ImageLogo, d, append(suffixed(b, "-logo"), suffixed(b, "-clearlogo")...)...)
		c.stems(metadata.ImageBanner, d, suffixed(b, "-banner")...)
		c.stems(metadata.ImageThumb, d, append(suffixed(b, "-landscape"), suffixed(b, "-thumb")...)...)
		return
	}
	c.stems(metadata.ImagePoster, d, "poster", "folder", "cover", "default", "movie")
	c.stems(metadata.ImagePoster, d, append(suffixed(b, "-poster"), b...)...)
	c.stems(metadata.ImageBackdrop, d, "fanart", "backdrop", "background")
	c.stems(metadata.ImageBackdrop, d, append(suffixed(b, "-fanart"), suffixed(b, "-backdrop")...)...)
	c.numbered(d)
	c.extraFanart(d)
	c.stems(metadata.ImageLogo, d, "logo", "clearlogo")
	c.stems(metadata.ImageLogo, d, append(suffixed(b, "-logo"), suffixed(b, "-clearlogo")...)...)
	c.stems(metadata.ImageBanner, d, "banner")
	c.stems(metadata.ImageBanner, d, suffixed(b, "-banner")...)
	c.stems(metadata.ImageThumb, d, "landscape", "thumb")
	c.stems(metadata.ImageThumb, d, append(suffixed(b, "-landscape"), suffixed(b, "-thumb")...)...)
}

// seasonNumber is q.IndexNumber, or the number parsed from the folder name
// when the query carries none.
func seasonNumber(q metadata.Query) int {
	if q.IndexNumber != 0 {
		return q.IndexNumber
	}
	if n, ok := naming.ParseSeasonFolder(filepath.Base(q.Path)); ok {
		return n
	}
	return 0
}

// seasonPrefixes returns the Kodi season art prefixes: "season01",
// "season1"; "season-specials", "season00", "season0" for specials.
func seasonPrefixes(n int) []string {
	if n == 0 {
		return []string{"season-specials", "season00", "season0"}
	}
	return []string{fmt.Sprintf("season%02d", n), fmt.Sprintf("season%d", n)}
}

func seasonImages(c *collector, q metadata.Query) {
	if !isDir(q.Path) {
		return
	}
	d := readDir(q.Path)
	parent := readDir(filepath.Dir(filepath.Clean(q.Path)))
	pre := seasonPrefixes(seasonNumber(q))
	pat := func(suffix string) []string { return suffixed(pre, suffix) }
	all := []string{"season-all"}

	c.stems(metadata.ImagePoster, d, "poster", "folder", "cover")
	c.stems(metadata.ImagePoster, d, pat("-poster")...)
	c.stems(metadata.ImagePoster, parent, pat("-poster")...)
	c.stems(metadata.ImagePoster, parent, pre...)
	c.stems(metadata.ImagePoster, parent, suffixed(all, "-poster")...)

	c.stems(metadata.ImageBackdrop, d, "fanart", "backdrop", "background")
	c.stems(metadata.ImageBackdrop, parent, pat("-fanart")...)
	c.stems(metadata.ImageBackdrop, parent, suffixed(all, "-fanart")...)

	c.stems(metadata.ImageBanner, d, "banner")
	c.stems(metadata.ImageBanner, parent, pat("-banner")...)
	c.stems(metadata.ImageBanner, parent, suffixed(all, "-banner")...)

	c.stems(metadata.ImageThumb, d, "landscape", "thumb")
	c.stems(metadata.ImageThumb, parent, pat("-landscape")...)
	c.stems(metadata.ImageThumb, parent, pat("-thumb")...)
	c.stems(metadata.ImageThumb, parent, suffixed(all, "-landscape")...)
}
