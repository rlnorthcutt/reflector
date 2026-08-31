package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBasicAuthRejectsMissingCredentials(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/basic-auth/alice/secret", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

func TestBasicAuthRejectsWrongCredentials(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/basic-auth/alice/secret", nil)
	req.SetBasicAuth("alice", "wrong")
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestBasicAuthAcceptsRightCredentials(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/basic-auth/alice/secret", nil)
	req.SetBasicAuth("alice", "secret")
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestBearerRejectsMissingToken(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/bearer", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestBearerRejectsEmptyToken(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/bearer", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestBearerAcceptsAnyNonEmptyToken(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/bearer", nil)
	req.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestCookiesRoundTrip(t *testing.T) {
	s := testServer()
	mux := s.builtinMux()

	setReq := httptest.NewRequest(http.MethodGet, "/cookies/set?a=1&b=2", nil)
	setW := httptest.NewRecorder()
	mux.ServeHTTP(setW, setReq)
	if setW.Code != http.StatusFound {
		t.Fatalf("cookies/set status = %d, want 302", setW.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/cookies", nil)
	for _, c := range setW.Result().Cookies() {
		getReq.AddCookie(c)
	}
	getW := httptest.NewRecorder()
	mux.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", getW.Code)
	}
	body := getW.Body.String()
	if !strings.Contains(body, "a=1") || !strings.Contains(body, "b=2") {
		t.Errorf("body = %q, want both cookies listed", body)
	}
}

func TestCookiesDeleteExpiresCookie(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/cookies/delete?a=", nil)
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Errorf("cookies = %+v, want one expired (negative MaxAge) cookie", cookies)
	}
}
