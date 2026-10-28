package refresh

import (
	"testing"

	"github.com/fs0ciety7000/aether-media/internal/metadata"
)

func TestMergePriority(t *testing.T) {
	nfo := &metadata.Result{
		Title:       "Le Fabuleux Destin d'Amélie Poulain",
		ProviderIDs: metadata.ProviderIDs{"imdb": "tt0211915"},
		Images:      []metadata.RemoteImage{{Type: "poster", URL: "/m/poster.jpg", Local: true}},
	}
	tmdb := &metadata.Result{
		Title:        "Amélie",
		Overview:     "Amélie, une jeune serveuse…",
		PremiereDate: "2001-04-25",
		Genres:       []string{"Comédie", "Romance"},
		ProviderIDs:  metadata.ProviderIDs{"tmdb": "194", "imdb": "tt9999999"},
		Images: []metadata.RemoteImage{
			{Type: "poster", URL: "https://x/p.jpg"},
			{Type: "backdrop", URL: "https://x/b1.jpg"}, {Type: "backdrop", URL: "https://x/b2.jpg"},
			{Type: "backdrop", URL: "https://x/b3.jpg"}, {Type: "backdrop", URL: "https://x/b4.jpg"},
			{Type: "backdrop", URL: "https://x/b5.jpg"}, {Type: "backdrop", URL: "https://x/b1.jpg"},
		},
		Children: map[int]*metadata.Result{1: {Title: "Pilote"}},
	}
	m := Merge(nfo, nil, tmdb)
	if m.Title != nfo.Title || m.Overview != tmdb.Overview || m.Year != 2001 {
		t.Errorf("scalars: %+v", m)
	}
	if m.ProviderIDs["imdb"] != "tt0211915" || m.ProviderIDs["tmdb"] != "194" {
		t.Errorf("ids: %v", m.ProviderIDs)
	}
	if len(m.Genres) != 2 {
		t.Errorf("genres: %v", m.Genres)
	}
	var posters, backdrops int
	for _, img := range m.Images {
		switch img.Type {
		case "poster":
			posters++
			if !img.Local {
				t.Error("local poster must win over remote")
			}
		case "backdrop":
			backdrops++
		}
	}
	if posters != 1 || backdrops != 4 {
		t.Errorf("images: %d posters, %d backdrops", posters, backdrops)
	}
	if m.Children[1].Title != "Pilote" {
		t.Errorf("children: %+v", m.Children)
	}
}
