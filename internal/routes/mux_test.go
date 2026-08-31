package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

func mustLoad(t *testing.T, fsys fstest.MapFS) []Route {
	t.Helper()
	list, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 0 {
		t.Fatalf("LoadDir errors: %v", errs)
	}
	return list
}

func testID() identity.Identity {
	return identity.Identity{Hostname: "web-2", InstanceID: "a1b2c3", Port: 8080, Version: "test"}
}

func TestMuxRendersInlineBodyTemplate(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte(`
routes:
  - path: /api/users/{id}
    method: GET
    response:
      body: |
        {"id":"{{ .PathParam "id" }}","served_by":"{{ .Hostname }}"}
`)},
	}
	mux, err := BuildMux(mustLoad(t, fsys), testID())
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users/42", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"id":"42"`) || !strings.Contains(w.Body.String(), `"served_by":"web-2"`) {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestMuxServesBodyFileWithInferredContentType(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /f\n    response:\n      body_file: payloads/f.json\n")},
		"payloads/f.json": {Data: []byte(`{"a":1}`)},
	}
	mux, err := BuildMux(mustLoad(t, fsys), testID())
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/f", nil))

	if w.Body.String() != `{"a":1}` {
		t.Errorf("body = %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestMuxExplicitHeaderOverridesInferredContentType(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte(`
routes:
  - path: /f
    response:
      headers:
        Content-Type: text/plain
      body_file: payloads/f.json
`)},
		"payloads/f.json": {Data: []byte(`{"a":1}`)},
	}
	mux, err := BuildMux(mustLoad(t, fsys), testID())
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/f", nil))

	if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain (explicit header wins)", ct)
	}
}

func TestMuxDefaultStatusIs200(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    response:\n      body: hi\n")},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestMuxExplicitStatus(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    response:\n      status: 201\n      body: hi\n")},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != 201 {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestMuxDelayWaits(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    response:\n      body: hi\n      delay: 50ms\n")},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	w := httptest.NewRecorder()
	start := time.Now()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 50ms", elapsed)
	}
}

func TestMuxFailureAlwaysFiresAtRateOne(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte(`
routes:
  - path: /x
    response:
      status: 200
      body: hi
      failure:
        rate: 1
        status: 503
`)},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != 503 {
		t.Errorf("status = %d, want 503 (failure always triggers at rate 1)", w.Code)
	}
}

func TestMuxBearerAuthRejectsMissingToken(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /secure\n    response:\n      body: hi\n      auth: bearer\n")},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/secure", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestMuxBearerAuthAcceptsToken(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /secure\n    response:\n      body: hi\n      auth: bearer\n")},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestMuxAnyMethodMatchesAllVerbs(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    response:\n      body: hi\n")},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, "/x", nil))
		if w.Code != 200 {
			t.Errorf("%s /x: status = %d, want 200", method, w.Code)
		}
	}
}

func TestMuxSpecificMethodRejectsOthers(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    method: POST\n    response:\n      body: hi\n")},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code == 200 {
		t.Error("GET matched a POST-only route")
	}
}

func TestMuxBodyJSONFromRequestBody(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte(`
routes:
  - path: /x
    method: POST
    response:
      body: "name={{ .BodyJSON.name }}"
`)},
	}
	mux, _ := BuildMux(mustLoad(t, fsys), testID())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"ada"}`))
	mux.ServeHTTP(w, req)
	if w.Body.String() != "name=ada" {
		t.Errorf("body = %q, want %q", w.Body.String(), "name=ada")
	}
}
