// Package webui serves the SvelteKit single-page application embedded in the
// binary. The SPA is built into ./dist by `make web`.
package webui

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var dist embed.FS

const placeholder = `<!doctype html><html lang="fr"><meta charset="utf-8"><title>Aether</title>
<body style="background:#121010;color:#EDE6DA;font:16px/1.6 system-ui;display:grid;place-items:center;height:100vh;margin:0">
<p>Interface web non compilée. Lancez <code>make web</code> puis recompilez le serveur.</p></body></html>`

// encodings are the precompressed variants written by the build
// (adapter-static `precompress`), in order of preference.
var encodings = []struct{ name, ext string }{{"br", ".br"}, {"gzip", ".gz"}}

// Handler serves static assets and falls back to index.html for client-side routes.
func Handler() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	return handler(root)
}

func handler(root fs.FS) http.Handler {
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		index = []byte(placeholder)
	}
	compressedIndex := map[string][]byte{}
	for _, e := range encodings {
		if b, err := fs.ReadFile(root, "index.html"+e.ext); err == nil {
			compressedIndex[e.name] = b
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(root, name); err == nil && !st.IsDir() {
				switch {
				case strings.HasPrefix(name, "_app/immutable/"):
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				case name == "service-worker.js" || name == "manifest.webmanifest" || name == "offline.html":
					// PWA: always revalidated, so a new version is picked up at once.
					w.Header().Set("Cache-Control", "no-cache")
				}
				serveFile(w, r, root, name)
				return
			}
			// Missing asset (has an extension): real 404, not the SPA shell.
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		h.Add("Vary", "Accept-Encoding")
		body := index
		if enc := pick(r, func(e string) bool { return compressedIndex[e] != nil }); enc != "" {
			h.Set("Content-Encoding", enc)
			body = compressedIndex[enc]
		}
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(body))
	})
}

// serveFile serves name, or its precompressed variant when the client
// accepts it. Range requests get the identity file.
func serveFile(w http.ResponseWriter, r *http.Request, root fs.FS, name string) {
	h := w.Header()
	ct := mime.TypeByExtension(path.Ext(name))
	if path.Ext(name) == ".webmanifest" {
		ct = "application/manifest+json"
	}
	if ct != "" {
		h.Set("Content-Type", ct)
	}
	served := name
	if compressible(name) {
		h.Add("Vary", "Accept-Encoding")
		if r.Header.Get("Range") == "" {
			if enc := pick(r, func(e string) bool { return exists(root, name+extOf(e)) }); enc != "" {
				h.Set("Content-Encoding", enc)
				served = name + extOf(enc)
			}
		}
	}
	f, err := root.Open(served)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rs, ok := f.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	})
	if !ok {
		http.Error(w, "unseekable asset", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, st.ModTime(), rs)
}

func compressible(name string) bool {
	switch path.Ext(name) {
	case ".js", ".mjs", ".css", ".html", ".json", ".svg", ".webmanifest", ".txt", ".xml":
		return true
	}
	return false
}

func extOf(enc string) string {
	for _, e := range encodings {
		if e.name == enc {
			return e.ext
		}
	}
	return ""
}

func exists(root fs.FS, name string) bool {
	st, err := fs.Stat(root, name)
	return err == nil && !st.IsDir()
}

// pick returns the preferred encoding accepted by the client (q > 0) for
// which have() is true, or "".
func pick(r *http.Request, have func(string) bool) string {
	accepted := map[string]bool{}
	for part := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.ReplaceAll(strings.TrimSpace(params), " ", "") == "q=0" {
			continue
		}
		accepted[strings.ToLower(strings.TrimSpace(name))] = true
	}
	for _, e := range encodings {
		if accepted[e.name] && have(e.name) {
			return e.name
		}
	}
	return ""
}
