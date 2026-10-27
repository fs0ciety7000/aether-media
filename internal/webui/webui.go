// Package webui serves the SvelteKit single-page application embedded in the
// binary. The SPA is built into ./dist by `make web`.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

const placeholder = `<!doctype html><html lang="fr"><meta charset="utf-8"><title>Aether</title>
<body style="background:#121010;color:#EDE6DA;font:16px/1.6 system-ui;display:grid;place-items:center;height:100vh;margin:0">
<p>Interface web non compilée. Lancez <code>make web</code> puis recompilez le serveur.</p></body></html>`

// Handler serves static assets and falls back to index.html for client-side routes.
func Handler() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		index = []byte(placeholder)
	}
	files := http.FileServerFS(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(root, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			// Missing asset (has an extension): real 404, not the SPA shell.
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
