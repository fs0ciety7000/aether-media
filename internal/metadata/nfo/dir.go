package nfo

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/fs0ciety7000/aether-media/internal/naming"
)

// dirIndex is a directory listing read once and matched case-insensitively.
type dirIndex struct {
	dir   string
	files map[string]string // lower-cased name -> real name
	dirs  map[string]string
	lower []string // lower-cased file names, sorted
}

// readDir lists dir; nil when it cannot be read.
func readDir(dir string) *dirIndex {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	idx := &dirIndex{dir: dir, files: map[string]string{}, dirs: map[string]string{}}
	for _, e := range entries {
		l := strings.ToLower(e.Name())
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(filepath.Join(dir, e.Name())); err == nil {
				isDir = fi.IsDir()
			}
		}
		if isDir {
			if _, ok := idx.dirs[l]; !ok {
				idx.dirs[l] = e.Name()
			}
			continue
		}
		if _, ok := idx.files[l]; !ok {
			idx.files[l] = e.Name()
			idx.lower = append(idx.lower, l)
		}
	}
	sort.Strings(idx.lower)
	return idx
}

// file returns the absolute path of the file named lower (case-insensitive).
func (d *dirIndex) file(lower string) string {
	if d == nil {
		return ""
	}
	if n, ok := d.files[lower]; ok {
		return filepath.Join(d.dir, n)
	}
	return ""
}

// sub returns the index of the sub-directory named lower.
func (d *dirIndex) sub(lower string) *dirIndex {
	if d == nil {
		return nil
	}
	if n, ok := d.dirs[lower]; ok {
		return readDir(filepath.Join(d.dir, n))
	}
	return nil
}

// stemOf returns a lower-cased file name without its extension.
func stemOf(name string) string {
	return strings.ToLower(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
}

var (
	stackSuffixRe  = regexp.MustCompile(`(?i)[\s._-]*(?:cd|dvd|part|pt|disc|disk)[\s._-]*(?:\d{1,2}|[a-d])$`)
	letterSuffixRe = regexp.MustCompile(`(?i)([\)\]])\s*-\s*[a-d]$`)
)

// unstack removes a multi-part marker: "heat (1995) cd1" -> "heat (1995)".
func unstack(stem string) string {
	s := stackSuffixRe.ReplaceAllString(stem, "")
	s = letterSuffixRe.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

// normTitle keeps lower-cased letters and digits only.
func normTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// movieDir describes a movie's folder: whether it is shared by several
// movies ("flat") and the lower-cased video base names of this movie
// (each stack part plus the name without the part marker).
type movieDir struct {
	idx   *dirIndex
	flat  bool
	bases []string
}

type movieGroup struct {
	title string
	year  int
	stems []string
}

// resolveMovie inspects a movie path (its folder, or one of its video files).
func resolveMovie(path, title string, year int) *movieDir {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	dir, file := path, ""
	if !fi.IsDir() {
		dir, file = filepath.Dir(path), path
	}
	idx := readDir(dir)
	if idx == nil {
		return nil
	}

	var groups []*movieGroup
	byKey := map[string]*movieGroup{}
	var fileGroup *movieGroup
	for _, l := range idx.lower {
		full := filepath.Join(dir, idx.files[l])
		if !naming.IsVideo(full) || naming.ClassifyExtra(full) != naming.ExtraNone {
			continue
		}
		m := naming.ParseMovie(full)
		key := normTitle(m.Title)
		if key == "" {
			key = normTitle(unstack(stemOf(l)))
		}
		key += "|" + strconv.Itoa(m.Year) // remakes share a title
		g := byKey[key]
		if g == nil {
			g = &movieGroup{title: m.Title, year: m.Year}
			byKey[key] = g
			groups = append(groups, g)
		}
		g.stems = append(g.stems, stemOf(l))
		if file != "" && strings.EqualFold(filepath.Base(file), idx.files[l]) {
			fileGroup = g
		}
	}

	md := &movieDir{idx: idx, flat: len(groups) > 1}
	var chosen *movieGroup
	switch {
	case fileGroup != nil:
		chosen = fileGroup
	case len(groups) == 1:
		chosen = groups[0]
	case len(groups) > 1:
		chosen = pickGroup(groups, title, year)
	}
	if file != "" && fileGroup == nil {
		md.bases = append(md.bases, stemOf(file))
	}
	if chosen != nil {
		seen := map[string]bool{}
		for _, s := range chosen.stems {
			for _, b := range []string{s, unstack(s)} {
				if b != "" && !seen[b] {
					seen[b] = true
					md.bases = append(md.bases, b)
				}
			}
		}
	}
	return md
}

// pickGroup finds the movie of a flat folder matching title (and year).
func pickGroup(groups []*movieGroup, title string, year int) *movieGroup {
	want := normTitle(title)
	if want == "" {
		return nil
	}
	var best *movieGroup
	bestScore := 0
	for _, g := range groups {
		score := 0
		switch {
		case normTitle(g.title) == want:
			score = 3
		default:
			for _, s := range g.stems {
				if strings.HasPrefix(normTitle(s), want) {
					score = 1
					break
				}
			}
		}
		if score == 0 {
			continue
		}
		if year != 0 && g.year != 0 {
			if g.year != year {
				continue
			}
			score++
		}
		if score > bestScore {
			best, bestScore = g, score
		}
	}
	return best
}
