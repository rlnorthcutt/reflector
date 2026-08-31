package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadRoutesFromDiskOnly(t *testing.T) {
	chdir(t, t.TempDir())
	writeFile(t, "routes.d/x.yaml", "routes:\n  - path: /x\n    response:\n      body: hi\n")

	rs := loadRoutes("routes.d", "")
	if len(rs.Errors) != 0 {
		t.Fatalf("Errors = %v", rs.Errors)
	}
	if len(rs.Resolved) != 1 || rs.Resolved[0].Path != "/x" {
		t.Fatalf("Resolved = %+v", rs.Resolved)
	}
}

func TestLoadRoutesFromPresetOnly(t *testing.T) {
	chdir(t, t.TempDir())

	rs := loadRoutes("routes.d", "flaky")
	if len(rs.Errors) != 0 {
		t.Fatalf("Errors = %v", rs.Errors)
	}
	found := false
	for _, r := range rs.Resolved {
		if r.Path == "/flaky" {
			found = true
		}
	}
	if !found {
		t.Errorf("Resolved = %+v, expected /flaky from the flaky preset", rs.Resolved)
	}
}

func TestLoadRoutesUserRouteShadowsPreset(t *testing.T) {
	chdir(t, t.TempDir())
	writeFile(t, "routes.d/override.yaml", "routes:\n  - path: /flaky\n    response:\n      body: overridden\n")

	rs := loadRoutes("routes.d", "flaky")
	if len(rs.Errors) != 0 {
		t.Fatalf("Errors = %v", rs.Errors)
	}
	for _, r := range rs.Resolved {
		if r.Path == "/flaky" && r.Source != "routes.d/override.yaml" {
			t.Errorf("route /flaky came from %q, want the user override", r.Source)
		}
	}
}

func TestLoadRoutesUnknownPresetIsAnError(t *testing.T) {
	chdir(t, t.TempDir())
	rs := loadRoutes("routes.d", "not-a-real-preset")
	if len(rs.Errors) != 1 {
		t.Fatalf("Errors = %v, want 1 unknown-preset error", rs.Errors)
	}
}

func TestParsePresetNamesTrimsAndSkipsEmpty(t *testing.T) {
	got := parsePresetNames(" rest-api ,, flaky ")
	want := []string{"rest-api", "flaky"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("parsePresetNames = %v, want %v", got, want)
	}
}
