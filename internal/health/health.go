// Package health tracks the shared healthy/unhealthy flag that backs
// /healthz and /readyz, toggleable live through the admin API to
// demonstrate load-balancer failover.
package health

import "sync/atomic"

// State holds the current health flag. The zero value is not ready to
// use; call New.
type State struct {
	healthy atomic.Bool
}

// New returns a State that starts healthy.
func New() *State {
	s := &State{}
	s.healthy.Store(true)
	return s
}

// Healthy reports the current flag.
func (s *State) Healthy() bool {
	return s.healthy.Load()
}

// SetHealthy updates the flag.
func (s *State) SetHealthy(v bool) {
	s.healthy.Store(v)
}
