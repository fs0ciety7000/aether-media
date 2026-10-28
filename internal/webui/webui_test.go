package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerFallback(t *testing.T) {
	h := Handler()
	cases := map[string]int{
		"/":                200,
		"/films/42":        200, // client-side route → index.html
		"/_app/missing.js": 404, // missing asset is not masked by the SPA
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s: status %d, want %d", path, rec.Code, want)
		}
	}
}

func TestPrecompressedAssets(t *testing.T) {
	root := fstest.MapFS{
		"index.html":               {Data: []byte("<!doctype html>shell")},
		"index.html.br":            {Data: []byte("BR-shell")},
		"index.html.gz":            {Data: []byte("GZ-shell")},
		"_app/immutable/app.js":    {Data: []byte("console.log(1)")},
		"_app/immutable/app.js.gz": {Data: []byte("GZ-js")},
		"icons/icon.png":           {Data: []byte("\x89PNG")},
	}
	h := handler(root)
	get := func(path, ae, rng string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if ae != "" {
			req.Header.Set("Accept-Encoding", ae)
		}
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	cases := []struct {
		path, ae, rng, enc, body, ctype string
	}{
		{"/", "gzip, deflate, br", "", "br", "BR-shell", "text/html; charset=utf-8"},
		{"/films/42", "gzip", "", "gzip", "GZ-shell", "text/html; charset=utf-8"},
		{"/", "", "", "", "<!doctype html>shell", "text/html; charset=utf-8"},
		{"/_app/immutable/app.js", "br, gzip", "", "gzip", "GZ-js", "text/javascript; charset=utf-8"},
		{"/_app/immutable/app.js", "br;q=1, gzip;q=0", "", "", "console.log(1)", "text/javascript; charset=utf-8"},
		{"/_app/immutable/app.js", "gzip", "bytes=0-6", "", "console", "text/javascript; charset=utf-8"},
		{"/icons/icon.png", "gzip, br", "", "", "\x89PNG", "image/png"},
	}
	for _, c := range cases {
		rec := get(c.path, c.ae, c.rng)
		if got := rec.Header().Get("Content-Encoding"); got != c.enc {
			t.Errorf("%s (%q): encoding %q, want %q", c.path, c.ae, got, c.enc)
		}
		if rec.Body.String() != c.body {
			t.Errorf("%s (%q): body %q, want %q", c.path, c.ae, rec.Body.String(), c.body)
		}
		if got := rec.Header().Get("Content-Type"); got != c.ctype {
			t.Errorf("%s: content-type %q, want %q", c.path, got, c.ctype)
		}
	}
	if rec := get("/_app/immutable/app.js", "gzip", ""); rec.Header().Get("Vary") != "Accept-Encoding" || rec.Header().Get("Cache-Control") == "" {
		t.Errorf("headers: %v", rec.Header())
	}
}
