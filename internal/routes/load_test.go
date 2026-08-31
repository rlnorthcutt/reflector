package routes

import (
	"testing"
	"testing/fstest"
)

func TestLoadDirMissingDirReturnsNoError(t *testing.T) {
	fsys := fstest.MapFS{}
	routes, errs := LoadDir(fsys, "routes.d")
	if len(routes) != 0 || len(errs) != 0 {
		t.Errorf("LoadDir(missing dir) = %v, %v; want empty, empty", routes, errs)
	}
}

func TestLoadDirParsesRoutesInFilenameOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/b.yaml": {Data: []byte(`
routes:
  - path: /b
    response:
      body: "b"
`)},
		"routes.d/a.yaml": {Data: []byte(`
routes:
  - path: /a
    response:
      body: "a"
`)},
	}
	list, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if len(list) != 2 || list[0].Path != "/a" || list[1].Path != "/b" {
		t.Fatalf("list = %+v, want [/a, /b] in that order", list)
	}
}

func TestLoadDirIgnoresNonYAMLFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/notes.txt": {Data: []byte("hello")},
		"routes.d/a.yaml":    {Data: []byte("routes:\n  - path: /a\n")},
	}
	list, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 0 || len(list) != 1 {
		t.Fatalf("list=%v errs=%v, want 1 route no errors", list, errs)
	}
}

func TestLoadDirReportsYAMLSyntaxError(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/bad.yaml": {Data: []byte("routes: [this is not valid yaml: :")},
	}
	_, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want 1 syntax error", errs)
	}
}

func TestCompileRejectsMissingPath(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - response:\n      body: hi\n")},
	}
	_, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want 1 error for missing path", errs)
	}
}

func TestCompileRejectsPathWithoutLeadingSlash(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: api/users\n")},
	}
	_, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want 1 error for path without leading slash", errs)
	}
}

func TestCompileRejectsProtectedPath(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /healthz\n")},
	}
	_, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want 1 error for protected path", errs)
	}
}

func TestCompileRejectsBodyAndBodyFileTogether(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte(`
routes:
  - path: /x
    response:
      body: "hi"
      body_file: payloads/x.txt
`)},
		"payloads/x.txt": {Data: []byte("hi")},
	}
	_, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want 1 error for body+body_file", errs)
	}
}

func TestCompileReadsBodyFileEagerly(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    response:\n      body_file: payloads/x.json\n")},
		"payloads/x.json": {Data: []byte(`{"ok":true}`)},
	}
	list, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if string(list[0].bodyFileBytes) != `{"ok":true}` {
		t.Errorf("bodyFileBytes = %q", list[0].bodyFileBytes)
	}
	if list[0].bodyFileContentType != "application/json" {
		t.Errorf("bodyFileContentType = %q, want application/json", list[0].bodyFileContentType)
	}
}

func TestCompileRejectsMissingBodyFile(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    response:\n      body_file: payloads/missing.json\n")},
	}
	_, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want 1 error for missing body_file", errs)
	}
}

func TestCompileRejectsInvalidTemplate(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    response:\n      body: \"{{ .Nope \"\n")},
	}
	_, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want 1 template parse error", errs)
	}
}

func TestCompileRejectsInvalidAuth(t *testing.T) {
	fsys := fstest.MapFS{
		"routes.d/x.yaml": {Data: []byte("routes:\n  - path: /x\n    response:\n      auth: basic\n")},
	}
	_, errs := LoadDir(fsys, "routes.d")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want 1 error for unsupported auth", errs)
	}
}

func TestResolveLaterFileWinsWithinUserRoutes(t *testing.T) {
	older := Route{Path: "/x", Source: "a.yaml", Status: 200}
	newer := Route{Path: "/x", Source: "b.yaml", Status: 201}
	resolved := Resolve([]Route{older, newer}, nil)
	if len(resolved) != 1 || resolved[0].Source != "b.yaml" {
		t.Fatalf("resolved = %+v, want single route from b.yaml", resolved)
	}
}

func TestResolveUserRoutesAlwaysWinOverPresets(t *testing.T) {
	user := Route{Path: "/x", Source: "routes.d/a.yaml", Status: 200}
	preset := Route{Path: "/x", Source: "preset:rest-api", Status: 999}
	resolved := Resolve([]Route{user}, []Route{preset})
	if len(resolved) != 1 || resolved[0].Source != "routes.d/a.yaml" {
		t.Fatalf("resolved = %+v, want user route to win", resolved)
	}
}

func TestResolveDifferentSpecificMethodsCoexist(t *testing.T) {
	get := Route{Path: "/x", Method: "GET", Source: "get.yaml"}
	post := Route{Path: "/x", Method: "POST", Source: "post.yaml"}
	resolved := Resolve([]Route{get, post}, nil)
	if len(resolved) != 2 {
		t.Fatalf("resolved = %+v, want GET and POST /x to coexist", resolved)
	}
}

func TestResolveAnyMethodOverridesSpecificPresetMethod(t *testing.T) {
	presetGet := Route{Path: "/flaky", Method: "GET", Source: "preset:flaky"}
	userAny := Route{Path: "/flaky", Method: "", Source: "routes.d/override.yaml"}
	resolved := Resolve([]Route{userAny}, []Route{presetGet})
	if len(resolved) != 1 || resolved[0].Source != "routes.d/override.yaml" {
		t.Fatalf("resolved = %+v, want the any-method user route to fully replace the preset's GET route", resolved)
	}
}

func TestResolveDistinctPathsBothKept(t *testing.T) {
	a := Route{Path: "/a"}
	b := Route{Path: "/b"}
	resolved := Resolve([]Route{a}, []Route{b})
	if len(resolved) != 2 {
		t.Fatalf("resolved = %+v, want both routes kept", resolved)
	}
}

func TestKeyNormalizesParamNames(t *testing.T) {
	a := Route{Path: "/status/{code}", Method: "GET"}
	b := Route{Path: "/status/{c}", Method: "GET"}
	if a.Key() != b.Key() {
		t.Errorf("Key() differ: %q vs %q, want equal (structurally same route)", a.Key(), b.Key())
	}
}

func TestPatternIncludesMethodOnlyWhenSet(t *testing.T) {
	any := Route{Path: "/x"}
	if any.Pattern() != "/x" {
		t.Errorf("Pattern() = %q, want /x", any.Pattern())
	}
	get := Route{Path: "/x", Method: "GET"}
	if get.Pattern() != "GET /x" {
		t.Errorf("Pattern() = %q, want \"GET /x\"", get.Pattern())
	}
}
