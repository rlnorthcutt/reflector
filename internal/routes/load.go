package routes

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// LoadDir loads and compiles every *.yaml/*.yml file directly inside dir
// on fsys, in filename order. It returns as many compiled routes as
// possible alongside any errors encountered, so callers (validate, in
// particular) can report every problem in one pass rather than stopping
// at the first.
func LoadDir(fsys fs.FS, dir string) ([]Route, []error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("reading %s: %w", dir, err)}
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := path.Ext(e.Name()); ext == ".yaml" || ext == ".yml" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var out []Route
	var errs []error
	for _, name := range names {
		file := path.Join(dir, name)
		routes, ferrs := loadFile(fsys, file)
		out = append(out, routes...)
		errs = append(errs, ferrs...)
	}
	return out, errs
}

func loadFile(fsys fs.FS, file string) ([]Route, []error) {
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, []error{fmt.Errorf("%s: %w", file, err)}
	}

	var rf rawFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, []error{fmt.Errorf("%s: %w", file, err)}
	}

	var out []Route
	var errs []error
	for i, raw := range rf.Routes {
		route, err := compile(fsys, raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: route %d (%s): %w", file, i, raw.Path, err))
			continue
		}
		route.Source = file
		out = append(out, route)
	}
	return out, errs
}

// compile reads body_file content immediately (from fsys, the same
// filesystem root the route file itself came from) rather than deferring
// to mux build time, so routes loaded from different roots — routes.d on
// disk, several embedded presets — can be freely combined into one route
// list afterward without losing track of where each body_file lives.
func compile(fsys fs.FS, raw rawRoute) (Route, error) {
	if raw.Path == "" {
		return Route{}, fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(raw.Path, "/") {
		return Route{}, fmt.Errorf("path %q must start with /", raw.Path)
	}
	if IsProtectedPath(raw.Path) {
		return Route{}, fmt.Errorf("path %q is reserved and cannot be overridden", raw.Path)
	}
	if raw.Response.Body != "" && raw.Response.BodyFile != "" {
		return Route{}, fmt.Errorf("body and body_file are mutually exclusive")
	}

	status := raw.Response.Status
	if status == 0 {
		status = 200
	} else if status < 100 || status > 599 {
		return Route{}, fmt.Errorf("status %d is not a valid HTTP status code", status)
	}

	auth := strings.ToLower(strings.TrimSpace(raw.Response.Auth))
	if auth != "" && auth != "bearer" {
		return Route{}, fmt.Errorf("auth %q is not supported (only \"bearer\" is)", raw.Response.Auth)
	}

	route := Route{
		Path:     raw.Path,
		Method:   strings.ToUpper(strings.TrimSpace(raw.Method)),
		Status:   status,
		Headers:  raw.Response.Headers,
		BodyFile: raw.Response.BodyFile,
		Auth:     auth,
	}

	if raw.Response.BodyFile != "" {
		data, err := fs.ReadFile(fsys, raw.Response.BodyFile)
		if err != nil {
			return Route{}, fmt.Errorf("reading body_file %q: %w", raw.Response.BodyFile, err)
		}
		route.bodyFileBytes = data
		route.bodyFileContentType = contentTypeForFile(raw.Response.BodyFile)
	}

	if raw.Response.Body != "" {
		tmpl, err := template.New(raw.Path).Funcs(FuncMap).Parse(raw.Response.Body)
		if err != nil {
			return Route{}, fmt.Errorf("body template: %w", err)
		}
		route.Body = tmpl
	}

	if raw.Response.Delay != "" {
		d, err := ParseDelay(raw.Response.Delay)
		if err != nil {
			return Route{}, err
		}
		route.Delay = d
	}

	if raw.Response.Failure != nil {
		f, err := newFailure(raw.Response.Failure)
		if err != nil {
			return Route{}, err
		}
		route.Failure = f
	}

	return route, nil
}

// Resolve combines routes.d entries and preset entries into a single,
// precedence-resolved list: within userRoutes, a later entry overlapping
// an earlier one (same path, overlapping method — see overlaps) replaces
// it; userRoutes always win over presetRoutes, even if the preset route
// is more specific; among presetRoutes, later entries win on overlap.
func Resolve(userRoutes, presetRoutes []Route) []Route {
	var ordered []Route
	fromUser := map[int]bool{}

	for _, r := range userRoutes {
		replaced := false
		for i, existing := range ordered {
			if overlaps(existing, r) {
				ordered[i] = r
				fromUser[i] = true
				replaced = true
				break
			}
		}
		if !replaced {
			fromUser[len(ordered)] = true
			ordered = append(ordered, r)
		}
	}

	for _, r := range presetRoutes {
		replaced := false
		skip := false
		for i, existing := range ordered {
			if overlaps(existing, r) {
				if fromUser[i] {
					skip = true // routes.d always wins over presets
					break
				}
				ordered[i] = r
				replaced = true
				break
			}
		}
		if !replaced && !skip {
			ordered = append(ordered, r)
		}
	}

	return ordered
}
