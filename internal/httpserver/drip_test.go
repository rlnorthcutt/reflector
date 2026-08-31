package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDripDefaults(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	start := time.Now()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/drip", nil))
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(w.Body.Bytes()) != 10 {
		t.Errorf("len(body) = %d, want 10 (default numbytes)", len(w.Body.Bytes()))
	}
	if elapsed < 1500*time.Millisecond {
		t.Errorf("elapsed = %v, want close to the 2s default duration", elapsed)
	}
}

func TestDripCustomParams(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	start := time.Now()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/drip?numbytes=5&duration=100ms&code=201", nil))
	elapsed := time.Since(start)

	if w.Code != 201 {
		t.Errorf("status = %d, want 201", w.Code)
	}
	if len(w.Body.Bytes()) != 5 {
		t.Errorf("len(body) = %d, want 5", len(w.Body.Bytes()))
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("elapsed = %v, want roughly 100ms", elapsed)
	}
}

func TestDripDelayAppliesBeforeBody(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	start := time.Now()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/drip?numbytes=1&duration=1ms&delay=100ms", nil))
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 100ms delay", elapsed)
	}
}

func TestDripRejectsInvalidParams(t *testing.T) {
	s := testServer()
	for _, q := range []string{"numbytes=-1", "numbytes=99999999", "code=999", "duration=bad", "delay=bad", "jitter=bad"} {
		w := httptest.NewRecorder()
		s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/drip?"+q, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("/drip?%s: status = %d, want 400", q, w.Code)
		}
	}
}

func TestDripZeroBytes(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/drip?numbytes=0", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(w.Body.Bytes()) != 0 {
		t.Errorf("len(body) = %d, want 0", len(w.Body.Bytes()))
	}
}
