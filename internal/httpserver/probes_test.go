package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbesReflectHealthState(t *testing.T) {
	s := testServer()
	mux := s.builtinMux()

	for _, path := range []string{"/healthz", "/readyz"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 while healthy", path, w.Code)
		}
	}

	s.Health.SetHealthy(false)

	for _, path := range []string{"/healthz", "/readyz"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503 while unhealthy", path, w.Code)
		}
	}
}
