package httpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOverrideStatusViaQuery(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	handler := withOverrides(inner)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?_status=503", nil))
	if w.Code != 503 {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestOverrideStatusViaHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	handler := withOverrides(inner)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Reflector-Status", "418")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 418 {
		t.Errorf("status = %d, want 418", w.Code)
	}
}

func TestOverrideBody(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("original"))
	})
	handler := withOverrides(inner)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?_body=replaced", nil))
	if w.Body.String() != "replaced" {
		t.Errorf("body = %q, want %q", w.Body.String(), "replaced")
	}
}

func TestOverrideDelayWaitsBeforeHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := withOverrides(inner)
	w := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?_delay=50ms", nil))
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 50ms", elapsed)
	}
}

func TestOverrideForcesConnectionClose(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	handler := withOverrides(inner)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x?_connection=close", nil))
	if w.Header().Get("Connection") != "close" {
		t.Errorf("Connection header = %q, want close", w.Header().Get("Connection"))
	}
}

func TestNoOverridesWhenNoneRequested(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(201)
		_, _ = w.Write([]byte("untouched"))
	})
	handler := withOverrides(inner)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !called || w.Code != 201 || w.Body.String() != "untouched" {
		t.Errorf("called=%v code=%d body=%q, want unmodified passthrough", called, w.Code, w.Body.String())
	}
}

// TestOverrideAbort uses a real httptest.Server because aborting relies
// on net/http's per-request panic recovery for http.ErrAbortHandler,
// which only kicks in on the real server plumbing, not a bare
// ResponseRecorder.
func TestOverrideAbort(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("should not be reached"))
	})
	srv := httptest.NewServer(withOverrides(inner))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/x?_abort=1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr == nil {
		t.Error("expected a read error from an aborted connection, got a clean read")
	}
	if string(body) != "partial response before simulated connection reset\n" {
		t.Errorf("partial body = %q, want the pre-abort message", body)
	}
}
