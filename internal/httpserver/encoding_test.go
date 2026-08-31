package httpserver

import (
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGzipRoute(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gzip", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if enc := w.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", enc)
	}

	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close()
	data, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("reading decompressed body: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body["encoding"] != "gzip" {
		t.Errorf("encoding field = %v, want \"gzip\"", body["encoding"])
	}
	if _, ok := body["server"]; !ok {
		t.Error("expected server section in decompressed envelope")
	}
}

func TestDeflateRoute(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deflate", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if enc := w.Header().Get("Content-Encoding"); enc != "deflate" {
		t.Errorf("Content-Encoding = %q, want deflate", enc)
	}

	fr := flate.NewReader(w.Body)
	defer fr.Close()
	data, err := io.ReadAll(fr)
	if err != nil {
		t.Fatalf("reading decompressed body: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body["encoding"] != "deflate" {
		t.Errorf("encoding field = %v, want \"deflate\"", body["encoding"])
	}
}

func TestGzipPoolReuseProducesValidOutputAcrossRequests(t *testing.T) {
	s := testServer()
	mux := s.builtinMux()
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gzip", nil))
		gr, err := gzip.NewReader(w.Body)
		if err != nil {
			t.Fatalf("request %d: gzip.NewReader: %v", i, err)
		}
		if _, err := io.ReadAll(gr); err != nil {
			t.Fatalf("request %d: reading: %v", i, err)
		}
		gr.Close()
	}
}
