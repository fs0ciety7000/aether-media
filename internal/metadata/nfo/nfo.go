// Package nfo is the local metadata provider: Kodi-style NFO files and
// artwork stored next to the media, following the Kodi, Jellyfin, Emby and
// tinyMediaManager conventions.
//
// Album artists have no dedicated Result field: they are returned in
// Result.People with Kind "artist" (MusicBrainz artist id in ProviderIDs).
// Remote artwork URLs listed inside the NFO are returned after the local
// files, with Local = false.
package nfo

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

// Provider reads NFO files and local images. It implements
// metadata.LocalProvider.
type Provider struct{}

var _ metadata.LocalProvider = (*Provider)(nil)

// New returns the local NFO / artwork provider.
func New() *Provider { return &Provider{} }

// Name implements metadata.LocalProvider.
func (*Provider) Name() string { return "nfo" }

// Supports implements metadata.LocalProvider.
func (*Provider) Supports(kind string) bool {
	switch kind {
	case metadata.KindMovie, metadata.KindSeries, metadata.KindSeason,
		metadata.KindEpisode, metadata.KindArtist, metadata.KindAlbum:
		return true
	}
	return false
}

// Local implements metadata.LocalProvider. It returns nil, nil when neither
// an NFO nor artwork is found. An unreadable or empty NFO is ignored; I/O
// errors other than "not found" are returned.
func (p *Provider) Local(ctx context.Context, q metadata.Query) (*metadata.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.Path == "" || !p.Supports(q.Kind) {
		return nil, nil
	}
	var res *metadata.Result
	if path := FindNFO(q); path != "" {
		r, err := parseFile(path, q.Kind)
		switch {
		case err == nil:
			res = r
		case errors.Is(err, ErrEmpty), errors.Is(err, fs.ErrNotExist):
		default:
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	imgs := FindImages(q)
	if res == nil && len(imgs) == 0 {
		return nil, nil
	}
	if res == nil {
		res = &metadata.Result{}
	}
	res.Images = append(imgs, res.Images...)
	return res, nil
}

func parseFile(path, kind string) (*metadata.Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return ParseNFO(f, kind)
}

// FindNFO returns the NFO file of an item, "" if none:
//   - movie: movie.nfo, <video>.nfo (also without the stack marker),
//     VIDEO_TS/VIDEO_TS.nfo, BDMV/index.nfo; in a folder shared by several
//     movies only <video>.nfo of the movie matching q.Title / q.Year;
//   - series: tvshow.nfo; season: season.nfo; episode: <video>.nfo;
//   - album: album.nfo; artist: artist.nfo.
func FindNFO(q metadata.Query) string {
	if q.Path == "" {
		return ""
	}
	switch q.Kind {
	case metadata.KindMovie:
		md := resolveMovie(q.Path, q.Title, q.Year)
		if md == nil {
			return ""
		}
		if !md.flat {
			if f := md.idx.file("movie.nfo"); f != "" {
				return f
			}
		}
		for _, b := range md.bases {
			if f := md.idx.file(b + ".nfo"); f != "" {
				return f
			}
		}
		if !md.flat {
			if f := md.idx.sub("video_ts").file("video_ts.nfo"); f != "" {
				return f
			}
			if f := md.idx.sub("bdmv").file("index.nfo"); f != "" {
				return f
			}
		}
	case metadata.KindEpisode:
		if isDir(q.Path) {
			return ""
		}
		return readDir(filepath.Dir(q.Path)).file(stemOf(q.Path) + ".nfo")
	case metadata.KindSeries:
		return folderNFO(q.Path, "tvshow.nfo")
	case metadata.KindSeason:
		return folderNFO(q.Path, "season.nfo")
	case metadata.KindAlbum:
		return folderNFO(q.Path, "album.nfo")
	case metadata.KindArtist:
		return folderNFO(q.Path, "artist.nfo")
	}
	return ""
}

func folderNFO(dir, name string) string {
	if !isDir(dir) {
		return ""
	}
	return readDir(dir).file(name)
}
