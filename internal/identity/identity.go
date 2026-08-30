// Package identity holds the per-process identity fields (hostname,
// instance ID, port, version) that every built-in response embeds so a
// client can tell which backend instance served a request.
package identity

import (
	"crypto/rand"
	"encoding/hex"
	"os"
)

// Version is the reflector build version. Overridden at build time via
// -ldflags "-X github.com/rlnorthcutt/reflector/internal/identity.Version=...".
var Version = "dev"

// Identity is the immutable server identity attached to every response.
type Identity struct {
	Hostname   string
	InstanceID string
	Port       int
	Version    string
}

// New builds an Identity for this process. hostnameOverride, if non-empty,
// takes precedence over the OS-reported hostname.
func New(hostnameOverride string, port int) Identity {
	hostname := hostnameOverride
	if hostname == "" {
		if h, err := os.Hostname(); err == nil {
			hostname = h
		} else {
			hostname = "unknown"
		}
	}

	return Identity{
		Hostname:   hostname,
		InstanceID: newInstanceID(),
		Port:       port,
		Version:    Version,
	}
}

// newInstanceID returns a short random hex identifier unique to this process
// run, e.g. "a1b2c3". It falls back to a fixed placeholder if the system
// random source is unavailable, which should not happen in practice.
func newInstanceID() string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}
	return hex.EncodeToString(buf)
}
