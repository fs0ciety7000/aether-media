package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
