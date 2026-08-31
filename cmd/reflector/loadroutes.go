package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	presets "github.com/rlnorthcutt/reflector/examples"
	"github.com/rlnorthcutt/reflector/internal/identity"
	"github.com/rlnorthcutt/reflector/internal/routes"
)

// routeSet holds every view of the loaded routes that the serve, routes,
// and validate subcommands each need.
type routeSet struct {
	User     []routes.Route // from routes.d on disk
	Presets  []routes.Route // from --preset packs
	Resolved []routes.Route // User and Presets combined at the correct precedence
	Errors   []error
}

// loadRoutes loads routes.d/*.yaml from disk and any named preset packs
// from the embedded examples filesystem, resolving them into one
// precedence-ordered list. It never returns a fatal error itself; load
// problems are collected in Errors so callers can decide how to react
// (fail startup, print and continue, etc).
func loadRoutes(routesDir, presetList string) routeSet {
	var rs routeSet

	rs.User, rs.Errors = routes.LoadDir(os.DirFS("."), routesDir)

	for _, name := range parsePresetNames(presetList) {
		if !isKnownPreset(name) {
			rs.Errors = append(rs.Errors, fmt.Errorf("unknown preset %q (available: %s)", name, strings.Join(presets.Names, ", ")))
			continue
		}
		sub, err := fs.Sub(presets.FS, name)
		if err != nil {
			rs.Errors = append(rs.Errors, fmt.Errorf("preset %q: %w", name, err))
			continue
		}
		r, errs := routes.LoadDir(sub, "routes.d")
		for i := range r {
			r[i].Source = "preset:" + name
		}
		rs.Presets = append(rs.Presets, r...)
		rs.Errors = append(rs.Errors, errs...)
	}

	rs.Resolved = routes.Resolve(rs.User, rs.Presets)
	return rs
}

// buildUserMux loads routes.d + presets and compiles them into a mux, in
// one call. It's used at startup and by both the admin reload endpoint
// and --watch, which all need the exact same "load, then build" sequence.
// mux is nil (with count 0) when there are no user/preset routes at all.
func buildUserMux(routesDir, presetList string, id identity.Identity) (mux *http.ServeMux, count int, err error) {
	rs := loadRoutes(routesDir, presetList)
	if len(rs.Errors) > 0 {
		return nil, 0, errors.Join(rs.Errors...)
	}
	if len(rs.Resolved) == 0 {
		return nil, 0, nil
	}
	mux, err = routes.BuildMux(rs.Resolved, id)
	if err != nil {
		return nil, 0, err
	}
	return mux, len(rs.Resolved), nil
}

func parsePresetNames(list string) []string {
	var names []string
	for _, n := range strings.Split(list, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			names = append(names, n)
		}
	}
	return names
}

func isKnownPreset(name string) bool {
	for _, n := range presets.Names {
		if n == name {
			return true
		}
	}
	return false
}
