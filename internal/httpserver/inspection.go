package httpserver

import (
	"net/http"

	"github.com/rlnorthcutt/reflector/internal/envelope"
	"github.com/rlnorthcutt/reflector/internal/negotiate"
)

// handleRoot serves the full envelope, content-negotiated between
// plaintext and JSON.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	env := envelope.Build(r, s.ID)
	negotiate.Write(w, r, http.StatusOK, env)
}

// handleAnything backs /echo and /anything/*: any verb, any subpath,
// full envelope including body.
func (s *Server) handleAnything(w http.ResponseWriter, r *http.Request) {
	env := envelope.Build(r, s.ID)
	negotiate.Write(w, r, http.StatusOK, env)
}

// handleHeaders returns just the request headers section of the envelope.
func (s *Server) handleHeaders(w http.ResponseWriter, r *http.Request) {
	view := envelope.HeadersView{Headers: map[string][]string(r.Header)}
	negotiate.Write(w, r, http.StatusOK, view)
}

// handleIP returns just the client IP.
func (s *Server) handleIP(w http.ResponseWriter, r *http.Request) {
	view := envelope.IPView{IP: envelope.ClientInfo(r).IP}
	negotiate.Write(w, r, http.StatusOK, view)
}

// handleUserAgent returns just the User-Agent header value.
func (s *Server) handleUserAgent(w http.ResponseWriter, r *http.Request) {
	view := envelope.UserAgentView{UserAgent: r.Header.Get("User-Agent")}
	negotiate.Write(w, r, http.StatusOK, view)
}
