// Package refresh orchestrates metadata providers: it identifies items,
// merges local and remote results, stores them and fetches artwork.
package refresh

import (
	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

// imageQuota caps how many images of each type are kept per item.
var imageQuota = map[string]int{
	metadata.ImagePoster:   1,
	metadata.ImageBackdrop: 4,
	metadata.ImageLogo:     1,
	metadata.ImageThumb:    1,
	metadata.ImageBanner:   1,
}

// Merge combines results in priority order (first = highest priority, e.g.
// the NFO before TMDB): scalar fields take the first non-zero value, lists
// the first non-empty list, provider ids are unioned (earlier wins on
// conflict) and images are concatenated by priority then capped per type.
func Merge(layers ...*metadata.Result) *metadata.Result {
	out := &metadata.Result{ProviderIDs: metadata.ProviderIDs{}}
	seenImg := map[string]bool{}
	perType := map[string]int{}
	for _, l := range layers {
		if l == nil {
			continue
		}
		first(&out.Title, l.Title)
		first(&out.OriginalTitle, l.OriginalTitle)
		first(&out.SortTitle, l.SortTitle)
		first(&out.Overview, l.Overview)
		first(&out.Tagline, l.Tagline)
		first(&out.PremiereDate, l.PremiereDate)
		first(&out.EndDate, l.EndDate)
		first(&out.Status, l.Status)
		first(&out.OfficialRating, l.OfficialRating)
		first(&out.Collection, l.Collection)
		first(&out.Year, l.Year)
		first(&out.RuntimeMs, l.RuntimeMs)
		first(&out.CommunityRating, l.CommunityRating)
		first(&out.IndexNumber, l.IndexNumber)
		first(&out.ParentIndex, l.ParentIndex)
		if len(out.Genres) == 0 {
			out.Genres = l.Genres
		}
		if len(out.Studios) == 0 {
			out.Studios = l.Studios
		}
		if len(out.People) == 0 {
			out.People = l.People
		}
		for k, v := range l.ProviderIDs {
			if _, ok := out.ProviderIDs[k]; !ok && v != "" {
				out.ProviderIDs[k] = v
			}
		}
		for _, img := range l.Images {
			if img.URL == "" || seenImg[img.URL] || perType[img.Type] >= quota(img.Type) {
				continue
			}
			seenImg[img.URL] = true
			perType[img.Type]++
			out.Images = append(out.Images, img)
		}
		for n, c := range l.Children {
			if out.Children == nil {
				out.Children = map[int]*metadata.Result{}
			}
			out.Children[n] = Merge(out.Children[n], c)
		}
	}
	if out.Year == 0 && len(out.PremiereDate) >= 4 {
		out.Year = atoi4(out.PremiereDate)
	}
	return out
}

func quota(t string) int {
	if q, ok := imageQuota[t]; ok {
		return q
	}
	return 1
}

func first[T comparable](dst *T, v T) {
	var zero T
	if *dst == zero {
		*dst = v
	}
}

func atoi4(s string) int {
	n := 0
	for _, c := range s[:4] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
