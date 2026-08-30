package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

func testServer() *Server {
	return &Server{
		ID: identity.Identity{
			Hostname:   "web-2",
			InstanceID: "a1b2c3",
			Port:       8080,
			Version:    "test",
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func doRequest(t *testing.T, mux http.Handler, method, target string, jsonAccept bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if jsonAccept {
		req.Header.Set("Accept", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestRoot(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/", true)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	server, ok := body["server"].(map[string]any)
	if !ok || server["hostname"] != "web-2" {
		t.Errorf("server section = %v", body["server"])
	}
}

func TestRootPlaintextByDefault(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/", false)

	if !strings.Contains(w.Body.String(), "Hostname: web-2") {
		t.Errorf("plaintext body missing hostname: %q", w.Body.String())
	}
}

func TestAnythingAcceptsAnyVerbAndSubpath(t *testing.T) {
	s := testServer()
	for _, target := range []string{"/echo", "/echo/foo", "/anything", "/anything/orders/42"} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			w := doRequest(t, s.routes(), method, target, true)
			if w.Code != http.StatusOK {
				t.Errorf("%s %s: status = %d, want 200", method, target, w.Code)
			}
		}
	}
}

func TestHeadersRoute(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/headers", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Test", "abc")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)

	var body struct {
		Headers map[string][]string `json:"headers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got := body.Headers["X-Test"]; len(got) != 1 || got[0] != "abc" {
		t.Errorf("Headers[X-Test] = %v", got)
	}
}

func TestIPRoute(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.Header.Set("Accept", "application/json")
	req.RemoteAddr = "192.0.2.1:4321"
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)

	var body struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body.IP != "192.0.2.1" {
		t.Errorf("IP = %q, want 192.0.2.1", body.IP)
	}
}

func TestUserAgentRoute(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/user-agent", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "test-agent/1.0")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)

	var body struct {
		UserAgent string `json:"user_agent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body.UserAgent != "test-agent/1.0" {
		t.Errorf("UserAgent = %q, want test-agent/1.0", body.UserAgent)
	}
}

func TestStatusRoute(t *testing.T) {
	s := testServer()
	tests := []struct {
		path string
		want int
	}{
		{"/status/200", 200},
		{"/status/503", 503},
		{"/status/418", 418},
	}
	for _, tt := range tests {
		w := doRequest(t, s.routes(), http.MethodGet, tt.path, false)
		if w.Code != tt.want {
			t.Errorf("%s: status = %d, want %d", tt.path, w.Code, tt.want)
		}
	}
}

func TestStatusRouteRejectsInvalidCode(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/status/notanumber", false)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDelayRouteWaitsAndResponds(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/delay/10ms", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
}

func TestDelayRouteRejectsInvalidDuration(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/delay/notaduration", false)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSizeRouteReturnsExactZeroFilledBytes(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/size/100", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(w.Body.Bytes()) != 100 {
		t.Errorf("len(body) = %d, want 100", len(w.Body.Bytes()))
	}
	for i, b := range w.Body.Bytes() {
		if b != 0 {
			t.Fatalf("byte %d = %d, want 0", i, b)
		}
	}
}

func TestBytesRouteWithoutSeedIsZeroFilled(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/bytes/50", false)
	if len(w.Body.Bytes()) != 50 {
		t.Errorf("len(body) = %d, want 50", len(w.Body.Bytes()))
	}
}

func TestBytesRouteWithSeedIsDeterministic(t *testing.T) {
	s := testServer()
	w1 := doRequest(t, s.routes(), http.MethodGet, "/bytes/64?seed=42", false)
	w2 := doRequest(t, s.routes(), http.MethodGet, "/bytes/64?seed=42", false)
	if w1.Body.String() != w2.Body.String() {
		t.Error("expected identical output for the same seed")
	}
	w3 := doRequest(t, s.routes(), http.MethodGet, "/bytes/64?seed=43", false)
	if w1.Body.String() == w3.Body.String() {
		t.Error("expected different output for different seeds")
	}
}

func TestBytesRouteAcrossChunkBoundary(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/bytes/70000?seed=1", false)
	if len(w.Body.Bytes()) != 70000 {
		t.Errorf("len(body) = %d, want 70000", len(w.Body.Bytes()))
	}
}

func TestSizeRouteRejectsTooLarge(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.routes(), http.MethodGet, "/size/99999999999999999999", false)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHealthzAndReadyz(t *testing.T) {
	s := testServer()
	for _, path := range []string{"/healthz", "/readyz"} {
		w := doRequest(t, s.routes(), http.MethodGet, path, false)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, w.Code)
		}
	}
}
