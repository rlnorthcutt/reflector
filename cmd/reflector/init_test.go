package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInitExtractsAllPresetsByDefault(t *testing.T) {
	chdir(t, t.TempDir())

	if err := runInit(nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	for _, want := range []string{"routes.d/rest-api.yaml", "routes.d/flaky.yaml", "payloads/users.json", "payloads/blob.bin"} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected %s to exist: %v", want, err)
		}
	}
}

func TestRunInitExtractsOnlyNamedPresets(t *testing.T) {
	chdir(t, t.TempDir())

	if err := runInit([]string{"flaky"}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if _, err := os.Stat("routes.d/flaky.yaml"); err != nil {
		t.Errorf("expected routes.d/flaky.yaml to exist: %v", err)
	}
	if _, err := os.Stat("routes.d/rest-api.yaml"); err == nil {
		t.Error("routes.d/rest-api.yaml should not exist (rest-api not requested)")
	}
}

func TestRunInitDoesNotOverwriteWithoutForce(t *testing.T) {
	chdir(t, t.TempDir())
	writeFile(t, "routes.d/flaky.yaml", "custom content\n")

	if err := runInit([]string{"flaky"}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	data, err := os.ReadFile("routes.d/flaky.yaml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "custom content\n" {
		t.Errorf("existing file was overwritten without --force")
	}
}

func TestRunInitForceOverwrites(t *testing.T) {
	chdir(t, t.TempDir())
	writeFile(t, "routes.d/flaky.yaml", "custom content\n")

	if err := runInit([]string{"--force", "flaky"}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	data, err := os.ReadFile("routes.d/flaky.yaml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) == "custom content\n" {
		t.Error("--force should have overwritten the existing file")
	}
}

func TestRunInitRejectsUnknownPreset(t *testing.T) {
	chdir(t, t.TempDir())
	if err := runInit([]string{"not-a-preset"}); err == nil {
		t.Error("expected error for unknown preset")
	}
	if _, err := os.Stat("routes.d"); err == nil {
		t.Error("routes.d should not have been created when validation failed")
	}
}

func TestRunInitExtractedRoutesValidate(t *testing.T) {
	chdir(t, t.TempDir())
	if err := runInit(nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if err := runValidate(nil); err != nil {
		t.Errorf("extracted presets failed validation: %v", err)
	}
	if _, err := os.Stat(filepath.Join("routes.d", "auth.yaml")); err != nil {
		t.Errorf("auth.yaml missing: %v", err)
	}
}
