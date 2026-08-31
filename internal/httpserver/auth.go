package httpserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rlnorthcutt/reflector/internal/negotiate"
)

type authView struct {
	Authenticated bool   `json:"authenticated"`
	User          string `json:"user,omitempty"`
	Token         string `json:"token,omitempty"`
}

func (v authView) PlainText() string {
	if v.User != "" {
		return fmt.Sprintf("authenticated: %t\nuser: %s\n", v.Authenticated, v.User)
	}
	if v.Token != "" {
		return fmt.Sprintf("authenticated: %t\ntoken: %s\n", v.Authenticated, v.Token)
	}
	return fmt.Sprintf("authenticated: %t\n", v.Authenticated)
}

// handleBasicAuth issues a 401 Basic challenge and validates credentials
// against the {user}/{pass} path values.
func (s *Server) handleBasicAuth(w http.ResponseWriter, r *http.Request) {
	wantUser := r.PathValue("user")
	wantPass := r.PathValue("pass")

	user, pass, ok := r.BasicAuth()
	if !ok || user != wantUser || pass != wantPass {
		w.Header().Set("WWW-Authenticate", `Basic realm="reflector"`)
		negotiate.Write(w, r, http.StatusUnauthorized, authView{Authenticated: false})
		return
	}
	negotiate.Write(w, r, http.StatusOK, authView{Authenticated: true, User: user})
}

// handleBearer validates presence (not value) of a bearer token.
func (s *Server) handleBearer(w http.ResponseWriter, r *http.Request) {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if len(auth) <= len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		negotiate.Write(w, r, http.StatusUnauthorized, authView{Authenticated: false})
		return
	}
	token := strings.TrimSpace(auth[len(prefix):])
	if token == "" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		negotiate.Write(w, r, http.StatusUnauthorized, authView{Authenticated: false})
		return
	}
	negotiate.Write(w, r, http.StatusOK, authView{Authenticated: true, Token: token})
}

type cookiesView struct {
	Cookies map[string]string `json:"cookies"`
}

func (v cookiesView) PlainText() string {
	var b strings.Builder
	for name, value := range v.Cookies {
		fmt.Fprintf(&b, "%s=%s\n", name, value)
	}
	return b.String()
}

// handleCookies returns the cookies the client sent.
func (s *Server) handleCookies(w http.ResponseWriter, r *http.Request) {
	cookies := map[string]string{}
	for _, c := range r.Cookies() {
		cookies[c.Name] = c.Value
	}
	negotiate.Write(w, r, http.StatusOK, cookiesView{Cookies: cookies})
}

// handleCookiesSet sets one cookie per query parameter, then redirects to
// /cookies so the round trip is visible.
func (s *Server) handleCookiesSet(w http.ResponseWriter, r *http.Request) {
	for name, values := range r.URL.Query() {
		value := ""
		if len(values) > 0 {
			value = values[0]
		}
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/"})
	}
	http.Redirect(w, r, "/cookies", http.StatusFound)
}

// handleCookiesDelete expires one cookie per query parameter (values are
// ignored; only the names matter), then redirects to /cookies.
func (s *Server) handleCookiesDelete(w http.ResponseWriter, r *http.Request) {
	for name := range r.URL.Query() {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1})
	}
	http.Redirect(w, r, "/cookies", http.StatusFound)
}
