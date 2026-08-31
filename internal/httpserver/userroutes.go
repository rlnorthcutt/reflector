package httpserver

import (
	"net/http"
	"sync/atomic"
)

// UserRoutes holds the currently active user/preset route mux behind an
// atomic pointer, so it can be swapped out at runtime — by
// POST /admin/routes/reload or --watch — without disrupting in-flight
// requests.
type UserRoutes struct {
	mux atomic.Pointer[http.ServeMux]
}

// NewUserRoutes wraps an initial mux (which may be nil, meaning no
// user/preset routes are loaded).
func NewUserRoutes(mux *http.ServeMux) *UserRoutes {
	ur := &UserRoutes{}
	ur.Store(mux)
	return ur
}

// Store atomically replaces the active mux.
func (u *UserRoutes) Store(mux *http.ServeMux) {
	u.mux.Store(mux)
}

// Load returns the currently active mux, or nil if none is loaded.
func (u *UserRoutes) Load() *http.ServeMux {
	if u == nil {
		return nil
	}
	return u.mux.Load()
}
