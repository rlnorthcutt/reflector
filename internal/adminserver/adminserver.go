// Package adminserver implements Reflector's admin API: health toggle,
// request capture access, and route table introspection/reload. It is
// served on its own port so it can be firewalled separately from traffic.
package adminserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/rlnorthcutt/reflector/internal/capture"
	"github.com/rlnorthcutt/reflector/internal/health"
)

// Default server-side timeouts, matching the main traffic server.
const (
	ReadTimeout  = 10 * time.Second
	WriteTimeout = 30 * time.Second
	IdleTimeout  = 120 * time.Second
)

// RouteInfo is one row of `GET /admin/routes` output.
type RouteInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Source string `json:"source"`
}

// Server holds the admin API's dependencies. Reload and ListRoutes are
// supplied by the caller (cmd/reflector) since they involve disk paths
// and preset loading that don't belong in this package.
type Server struct {
	Health     *health.State
	Capture    *capture.Buffer // nil disables the /admin/requests endpoints
	Reload     func() (routeCount int, err error)
	ListRoutes func() []RouteInfo
}

// New builds an http.Server bound to addr serving the admin API.
func New(addr string, s *Server, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/health/down", s.handleHealthDown)
	mux.HandleFunc("POST /admin/health/up", s.handleHealthUp)
	mux.HandleFunc("GET /admin/requests", s.handleGetRequests)
	mux.HandleFunc("DELETE /admin/requests", s.handleDeleteRequests)
	mux.HandleFunc("POST /admin/routes/reload", s.handleReload)
	mux.HandleFunc("GET /admin/routes", s.handleListRoutes)

	return &http.Server{
		Addr:         addr,
		Handler:      withLogging(mux, logger),
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
		IdleTimeout:  IdleTimeout,
	}
}

func (s *Server) handleHealthDown(w http.ResponseWriter, r *http.Request) {
	s.Health.SetHealthy(false)
	writeOK(w)
}

func (s *Server) handleHealthUp(w http.ResponseWriter, r *http.Request) {
	s.Health.SetHealthy(true)
	writeOK(w)
}

func (s *Server) handleGetRequests(w http.ResponseWriter, r *http.Request) {
	if s.Capture == nil {
		http.Error(w, "request capture is disabled (start with --capture)", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.Capture.All())
}

func (s *Server) handleDeleteRequests(w http.ResponseWriter, r *http.Request) {
	if s.Capture == nil {
		http.Error(w, "request capture is disabled (start with --capture)", http.StatusNotFound)
		return
	}
	s.Capture.Clear()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if s.Reload == nil {
		http.Error(w, "reload not available", http.StatusNotImplemented)
		return
	}
	count, err := s.Reload()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"reloaded": true, "routes": count})
}

func (s *Server) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	var list []RouteInfo
	if s.ListRoutes != nil {
		list = s.ListRoutes()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func withLogging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("admin request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
