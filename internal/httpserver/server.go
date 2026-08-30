// Package httpserver assembles Reflector's built-in HTTP routes into an
// http.Server with sane default timeouts.
package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

// Default server-side timeouts, always on per PLAN.md's performance
// requirements.
const (
	ReadTimeout  = 10 * time.Second
	WriteTimeout = 30 * time.Second
	IdleTimeout  = 120 * time.Second
)

// Server holds the dependencies built-in route handlers need.
type Server struct {
	ID     identity.Identity
	Logger *slog.Logger
}

// New builds an http.Server bound to addr, serving all built-in routes.
// When quiet is true, per-request access logging is suppressed.
func New(addr string, id identity.Identity, logger *slog.Logger, quiet bool) *http.Server {
	s := &Server{ID: id, Logger: logger}

	handler := s.routes()
	if !quiet {
		handler = withLogging(handler, logger)
	}

	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
		IdleTimeout:  IdleTimeout,
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/echo", s.handleAnything)
	mux.HandleFunc("/echo/", s.handleAnything)
	mux.HandleFunc("/anything", s.handleAnything)
	mux.HandleFunc("/anything/", s.handleAnything)
	mux.HandleFunc("/headers", s.handleHeaders)
	mux.HandleFunc("/ip", s.handleIP)
	mux.HandleFunc("/user-agent", s.handleUserAgent)

	mux.HandleFunc("/status/{code}", s.handleStatus)
	mux.HandleFunc("/delay/{duration}", s.handleDelay)

	mux.HandleFunc("/size/{bytes}", s.handleSize)
	mux.HandleFunc("/bytes/{n}", s.handleBytes)

	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)

	return mux
}

func withLogging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
