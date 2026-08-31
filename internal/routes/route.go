// Package routes implements Reflector's user-defined route engine: YAML
// route files, precedence/shadowing rules, and template rendering.
package routes

import (
	"fmt"
	"strings"
	"text/template"
)

// rawFile mirrors the on-disk YAML schema for a routes.d/*.yaml file.
type rawFile struct {
	Routes []rawRoute `yaml:"routes"`
}

type rawRoute struct {
	Path     string      `yaml:"path"`
	Method   string      `yaml:"method"`
	Response rawResponse `yaml:"response"`
}

type rawResponse struct {
	Status   int               `yaml:"status"`
	Headers  map[string]string `yaml:"headers"`
	Body     string            `yaml:"body"`
	BodyFile string            `yaml:"body_file"`
	Delay    string            `yaml:"delay"`
	Failure  *rawFailure       `yaml:"failure"`
	Auth     string            `yaml:"auth"`
}

type rawFailure struct {
	Rate   float64 `yaml:"rate"`
	Status int     `yaml:"status"`
}

// Route is a compiled, ready-to-serve route definition.
type Route struct {
	Path     string
	Method   string // "" means any method
	Status   int
	Headers  map[string]string
	Body     *template.Template // nil if BodyFile is set instead
	BodyFile string             // original path, for display in `reflector routes`
	Delay    *Delay
	Failure  *Failure
	Auth     string // "" or "bearer"

	// Source identifies where this route came from, for `reflector routes`
	// output, e.g. "routes.d/users-api.yaml" or "preset:rest-api".
	Source string

	bodyFileBytes       []byte
	bodyFileContentType string
}

// Key returns a precedence/shadowing key: method plus path with every
// {param} segment normalized, so routes that differ only in a path
// parameter's name are treated as the same slot.
func (r Route) Key() string {
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if method == "" {
		method = "ANY"
	}
	return method + " " + normalizePath(r.Path)
}

// normalizePath replaces every {param} segment with a fixed placeholder,
// so paths that differ only in a path parameter's name compare equal.
func normalizePath(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if len(s) >= 2 && strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			segs[i] = "{}"
		}
	}
	return strings.Join(segs, "/")
}

// overlaps reports whether a and b would both try to handle some of the
// same requests: same (normalized) path, and methods that aren't both
// specific-and-different. A route with no method set matches any method,
// so it overlaps every route at the same path regardless of that route's
// method.
func overlaps(a, b Route) bool {
	if normalizePath(a.Path) != normalizePath(b.Path) {
		return false
	}
	return a.Method == "" || b.Method == "" || strings.EqualFold(a.Method, b.Method)
}

// Pattern returns the http.ServeMux registration pattern for this route,
// e.g. "GET /api/users/{id}" or "/api/users/{id}" for any method.
func (r Route) Pattern() string {
	if r.Method == "" {
		return r.Path
	}
	return fmt.Sprintf("%s %s", strings.ToUpper(r.Method), r.Path)
}
