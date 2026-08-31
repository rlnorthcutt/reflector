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

// Options configures how built-in routes compose with user/preset routes.
type Options struct {
	// UserMux is the combined routes.d + preset route table, already
	// precedence-resolved. Nil means no user/preset routes are loaded.
	UserMux *http.ServeMux
	// StrictBuiltins, when true, makes built-ins win over any
	// user/preset route that would otherwise shadow them.
	StrictBuiltins bool
}

// New builds an http.Server bound to addr, serving all built-in routes
// plus, if configured, user/preset routes at the correct precedence. When
// quiet is true, per-request access logging is suppressed.
func New(addr string, id identity.Identity, logger *slog.Logger, quiet bool, opts Options) *http.Server {
	s := &Server{ID: id, Logger: logger}

	var handler http.Handler = s.builtinMux()
	if opts.UserMux != nil {
		handler = compose(s.builtinMux(), opts.UserMux, opts.StrictBuiltins)
	}
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

// BuiltinPatterns lists the built-in routes' registration patterns, for
// `reflector routes` output.
func BuiltinPatterns() []string {
	return []string{
		"/", "/echo", "/echo/*", "/anything", "/anything/*",
		"/headers", "/ip", "/user-agent",
		"/status/{code}", "/delay/{duration}",
		"/size/{bytes}", "/bytes/{n}",
		"/healthz", "/readyz",
	}
}

func (s *Server) builtinMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/{$}", s.handleRoot)
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

// compose returns a handler that tries first, then second, serving from
// whichever actually has a registered pattern matching the request.
// Neither matching results in a 404.
//
// Matching is done via Handler(r), which — per its documentation — does
// NOT populate named path wildcards. Dispatch is therefore always done
// via the matched mux's own ServeHTTP, which re-matches and correctly
// sets path values on the request before invoking the handler.
func compose(builtin, user *http.ServeMux, strictBuiltins bool) http.Handler {
	first, second := user, builtin
	if strictBuiltins {
		first, second = builtin, user
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := first.Handler(r); pattern != "" {
			first.ServeHTTP(w, r)
			return
		}
		if _, pattern := second.Handler(r); pattern != "" {
			second.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
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
