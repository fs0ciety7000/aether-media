package api

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompressJSON(t *testing.T) {
	big := strings.Repeat("Aether ", 2000)
	h := compressJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/json":
			writeJSON(w, 200, map[string]string{"v": big})
		case "/api/v1/empty":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/media":
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", "5")
			_, _ = io.WriteString(w, "video")
		case "/api/v1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			_, _ = io.WriteString(w, "data: 1\n\n")
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("flush: %v", err)
			}
			<-r.Context().Done()
		default:
			writeJSON(w, 200, map[string]string{"v": big})
		}
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	request := func(path, ae string) *http.Request {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		if ae != "" {
			req.Header.Set("Accept-Encoding", ae) // disables the transport's transparent gzip
		}
		return req
	}
	// fetch returns the headers and the raw (still encoded) body.
	fetch := func(path, ae string) (http.Header, []byte) {
		res, err := http.DefaultTransport.RoundTrip(request(path, ae))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.Header, body
	}

	hdr, body := fetch("/api/v1/json", "gzip, br")
	if hdr.Get("Content-Encoding") != "gzip" || hdr.Get("Vary") != "Accept-Encoding" {
		t.Fatalf("headers %v", hdr)
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]string
	if err := json.NewDecoder(zr).Decode(&v); err != nil || v["v"] != big {
		t.Fatalf("decode: %v", err)
	}

	for _, c := range []struct{ path, ae string }{
		{"/api/v1/json", "identity"}, {"/api/v1/json", "gzip;q=0"}, {"/api/v1/media", "gzip"},
		{"/api/v1/empty", "gzip"}, {"/static.json", "gzip"},
	} {
		if h, _ := fetch(c.path, c.ae); h.Get("Content-Encoding") != "" {
			t.Errorf("%s (%s): compressed", c.path, c.ae)
		}
	}
	if h, _ := fetch("/api/v1/media", "gzip"); h.Get("Content-Length") != "5" {
		t.Errorf("media Content-Length dropped: %v", h)
	}

	// SSE: not compressed, and the first event arrives before the stream ends.
	res, err := http.DefaultTransport.RoundTrip(request("/api/v1/events", "gzip")) //nolint:bodyclose // closed below (stream)
	if err != nil {
		t.Fatal(err)
	}
	stream := res.Body
	defer stream.Close()
	if res.Header.Get("Content-Encoding") != "" {
		t.Fatal("SSE compressed")
	}
	line := make(chan string, 1)
	go func() { l, _ := bufio.NewReader(stream).ReadString('\n'); line <- l }()
	select {
	case l := <-line:
		if l != "data: 1\n" {
			t.Fatalf("got %q", l)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SSE event not flushed")
	}
}
