package adminserver

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rlnorthcutt/reflector/internal/capture"
	"github.com/rlnorthcutt/reflector/internal/health"
)

func testMux(s *Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/health/down", s.handleHealthDown)
	mux.HandleFunc("POST /admin/health/up", s.handleHealthUp)
	mux.HandleFunc("GET /admin/requests", s.handleGetRequests)
	mux.HandleFunc("DELETE /admin/requests", s.handleDeleteRequests)
	mux.HandleFunc("POST /admin/routes/reload", s.handleReload)
	mux.HandleFunc("GET /admin/routes", s.handleListRoutes)
	return mux
}

func TestHealthToggle(t *testing.T) {
	h := health.New()
	s := &Server{Health: h}
	mux := testMux(s)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/health/down", nil))
	if w.Code != http.StatusOK || h.Healthy() {
		t.Errorf("after health/down: code=%d healthy=%v, want 200 and unhealthy", w.Code, h.Healthy())
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/health/up", nil))
	if w.Code != http.StatusOK || !h.Healthy() {
		t.Errorf("after health/up: code=%d healthy=%v, want 200 and healthy", w.Code, h.Healthy())
	}
}

func TestRequestsDisabledWithoutCapture(t *testing.T) {
	s := &Server{Health: health.New()}
	mux := testMux(s)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/requests", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /admin/requests without capture: status = %d, want 404", w.Code)
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/admin/requests", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("DELETE /admin/requests without capture: status = %d, want 404", w.Code)
	}
}

func TestRequestsReturnsCapturedEntries(t *testing.T) {
	buf := capture.NewBuffer(10)
	buf.Add(capture.Entry{Path: "/x"})
	s := &Server{Health: health.New(), Capture: buf}
	mux := testMux(s)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/requests", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var entries []capture.Entry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "/x" {
		t.Errorf("entries = %+v", entries)
	}
}

func TestDeleteRequestsClearsBuffer(t *testing.T) {
	buf := capture.NewBuffer(10)
	buf.Add(capture.Entry{Path: "/x"})
	s := &Server{Health: health.New(), Capture: buf}
	mux := testMux(s)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/admin/requests", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if len(buf.All()) != 0 {
		t.Error("expected buffer to be cleared")
	}
}

func TestReloadNotConfigured(t *testing.T) {
	s := &Server{Health: health.New()}
	mux := testMux(s)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/routes/reload", nil))
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", w.Code)
	}
}

func TestReloadSuccess(t *testing.T) {
	s := &Server{
		Health: health.New(),
		Reload: func() (int, error) { return 3, nil },
	}
	mux := testMux(s)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/routes/reload", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body["routes"] != float64(3) {
		t.Errorf("routes = %v, want 3", body["routes"])
	}
}

func TestReloadFailure(t *testing.T) {
	s := &Server{
		Health: health.New(),
		Reload: func() (int, error) { return 0, errors.New("bad route") },
	}
	mux := testMux(s)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/routes/reload", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListRoutes(t *testing.T) {
	s := &Server{
		Health: health.New(),
		ListRoutes: func() []RouteInfo {
			return []RouteInfo{{Method: "GET", Path: "/x", Status: 200, Source: "routes.d/x.yaml"}}
		},
	}
	mux := testMux(s)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/routes", nil))
	var list []RouteInfo
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(list) != 1 || list[0].Path != "/x" {
		t.Errorf("list = %+v", list)
	}
}

func TestListRoutesNilFuncReturnsEmptyList(t *testing.T) {
	s := &Server{Health: health.New()}
	mux := testMux(s)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/routes", nil))
	if w.Body.String() != "null\n" && w.Body.String() != "[]\n" {
		t.Errorf("body = %q, want an empty JSON list", w.Body.String())
	}
}

func TestNewBuildsAWorkingServer(t *testing.T) {
	s := New("127.0.0.1:0", &Server{Health: health.New()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if s.Handler == nil {
		t.Error("expected a non-nil handler")
	}
}
