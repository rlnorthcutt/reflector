package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestComposePopulatesPathValues guards against a real bug: ServeMux.Handler
// explicitly does not populate named path wildcards, so compose must
// dispatch via the matched mux's ServeHTTP (which re-matches and sets
// them), not by invoking the returned Handler directly.
func TestComposePopulatesPathValues(t *testing.T) {
	user := http.NewServeMux()
	user.HandleFunc("/api/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.PathValue("id")))
	})
	builtin := http.NewServeMux()

	handler := compose(builtin, NewUserRoutes(user), false)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users/42", nil))

	if w.Body.String() != "42" {
		t.Errorf("body = %q, want %q (path value must be populated)", w.Body.String(), "42")
	}
}

func TestComposeUserRouteShadowsBuiltinByDefault(t *testing.T) {
	user := http.NewServeMux()
	user.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("user"))
	})
	builtin := http.NewServeMux()
	builtin.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("builtin"))
	})

	handler := compose(builtin, NewUserRoutes(user), false)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/headers", nil))

	if w.Body.String() != "user" {
		t.Errorf("body = %q, want %q (user route should shadow builtin)", w.Body.String(), "user")
	}
}

func TestComposeStrictBuiltinsWins(t *testing.T) {
	user := http.NewServeMux()
	user.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("user"))
	})
	builtin := http.NewServeMux()
	builtin.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("builtin"))
	})

	handler := compose(builtin, NewUserRoutes(user), true)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/headers", nil))

	if w.Body.String() != "builtin" {
		t.Errorf("body = %q, want %q (--strict-builtins should make builtin win)", w.Body.String(), "builtin")
	}
}

func TestComposeFallsThroughToSecondOnNoMatch(t *testing.T) {
	user := http.NewServeMux()
	builtin := http.NewServeMux()
	builtin.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	handler := compose(builtin, NewUserRoutes(user), false)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", w.Body.String(), "ok")
	}
}

func TestComposeHandlesNilUserRoutes(t *testing.T) {
	builtin := http.NewServeMux()
	builtin.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	handler := compose(builtin, nil, false)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", w.Body.String(), "ok")
	}
}

func TestComposeReturns404WhenNeitherMatches(t *testing.T) {
	user := http.NewServeMux()
	builtin := http.NewServeMux()

	handler := compose(builtin, NewUserRoutes(user), false)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// TestRootIsExactMatchOnly guards against a v0.1 bug where "/" was
// registered as a subtree pattern and silently caught every unmatched
// path instead of 404ing.
func TestRootIsExactMatchOnly(t *testing.T) {
	s := testServer()
	w := httptest.NewRecorder()
	s.builtinMux().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/this-path-does-not-exist", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for unmatched path", w.Code)
	}
}
