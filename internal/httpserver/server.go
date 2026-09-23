// Package httpserver assembles Reflector's built-in HTTP routes into an
// http.Server with sane default timeouts.
package httpserver

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/rlnorthcutt/reflector/internal/capture"
	"github.com/rlnorthcutt/reflector/internal/health"
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
	Health *health.State
}

// Options configures how built-in routes compose with user/preset routes
// and other optional behavior.
type Options struct {
	// UserRoutes is the combined routes.d + preset route table, already
	// precedence-resolved. Nil, or a UserRoutes currently holding a nil
	// mux, means no user/preset routes are loaded. Swappable at runtime.
	UserRoutes *UserRoutes
	// StrictBuiltins, when true, makes built-ins win over any
	// user/preset route that would otherwise shadow them.
	StrictBuiltins bool
	// Health backs /healthz and /readyz. A new, healthy State is created
	// if this is nil.
	Health *health.State
	// Capture, if non-nil, records every request into its ring buffer.
	// Nil disables request capture (the default).
	Capture *capture.Buffer
	// NoOverrides disables per-request overrides
	// (_status/_delay/_body/_connection/_abort). They're honored by
	// default.
	NoOverrides bool
}

// New builds an http.Server bound to addr, serving all built-in routes
// plus, if configured, user/preset routes at the correct precedence. When
// quiet is true, per-request access logging is suppressed.
func New(addr string, id identity.Identity, logger *slog.Logger, quiet bool, opts Options) *http.Server {
	h := opts.Health
	if h == nil {
		h = health.New()
	}
	s := &Server{ID: id, Logger: logger, Health: h}

	var handler http.Handler = compose(s.builtinMux(), opts.UserRoutes, opts.StrictBuiltins)
	if !opts.NoOverrides {
		handler = withOverrides(handler)
	}
	if !quiet {
		handler = withLogging(handler, logger)
	}
	handler = capture.Middleware(opts.Capture, handler)

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
		"/status/{code}", "/delay/{duration}", "/drip",
		"/size/{bytes}", "/bytes/{n}", "/stream-bytes/{n}",
		"/basic-auth/{user}/{pass}", "/bearer",
		"/cookies", "/cookies/set", "/cookies/delete",
		"/gzip", "/deflate",
		"/ws",
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
	mux.HandleFunc("/drip", s.handleDrip)

	mux.HandleFunc("/size/{bytes}", s.handleSize)
	mux.HandleFunc("/bytes/{n}", s.handleBytes)
	mux.HandleFunc("/stream-bytes/{n}", s.handleStreamBytes)

	mux.HandleFunc("/basic-auth/{user}/{pass}", s.handleBasicAuth)
	mux.HandleFunc("/bearer", s.handleBearer)
	mux.HandleFunc("/cookies", s.handleCookies)
	mux.HandleFunc("/cookies/set", s.handleCookiesSet)
	mux.HandleFunc("/cookies/delete", s.handleCookiesDelete)

	mux.HandleFunc("/gzip", s.handleGzip)
	mux.HandleFunc("/deflate", s.handleDeflate)

	mux.HandleFunc("/ws", s.handleWS)

	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)

	return mux
}

// compose returns a handler that tries the user/preset mux (freshly
// loaded from userRoutes on every request, so admin reload and --watch
// take effect immediately) and the builtin mux in the precedence order
// StrictBuiltins implies, serving from whichever actually has a
// registered pattern matching the request. Neither matching results in a
// 404.
//
// Matching is done via Handler(r), which — per its documentation — does
// NOT populate named path wildcards. Dispatch is therefore always done
// via the matched mux's own ServeHTTP, which re-matches and correctly
// sets path values on the request before invoking the handler.
func compose(builtin *http.ServeMux, userRoutes *UserRoutes, strictBuiltins bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := userRoutes.Load()

		first, second := user, builtin
		if strictBuiltins {
			first, second = builtin, user
		}

		if first != nil {
			if _, pattern := first.Handler(r); pattern != "" {
				first.ServeHTTP(w, r)
				return
			}
		}
		if second != nil {
			if _, pattern := second.Handler(r); pattern != "" {
				second.ServeHTTP(w, r)
				return
			}
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

// Hijack forwards to the underlying ResponseWriter's Hijack. Without this,
// handlers that take over the connection (like /ws) would lose hijacking
// support whenever access logging is enabled: embedding an interface value
// only promotes the methods declared on that interface's static type
// (http.ResponseWriter), not Hijack, even though the concrete value
// underneath satisfies http.Hijacker too.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return hj.Hijack()
}
