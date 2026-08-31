package main

import "testing"

func TestRunValidateSucceedsOnCleanRoutes(t *testing.T) {
	chdir(t, t.TempDir())
	writeFile(t, "routes.d/x.yaml", "routes:\n  - path: /x\n    response:\n      body: hi\n")

	if err := runValidate(nil); err != nil {
		t.Errorf("runValidate: %v", err)
	}
}

func TestRunValidateFailsOnBadRoute(t *testing.T) {
	chdir(t, t.TempDir())
	writeFile(t, "routes.d/x.yaml", "routes:\n  - path: nope\n")

	if err := runValidate(nil); err == nil {
		t.Error("runValidate: expected error for path missing leading slash")
	}
}

func TestRunValidateOnCustomDir(t *testing.T) {
	chdir(t, t.TempDir())
	writeFile(t, "custom/x.yaml", "routes:\n  - path: /x\n")

	if err := runValidate([]string{"custom"}); err != nil {
		t.Errorf("runValidate([custom]): %v", err)
	}
}

func TestRunValidateEmptyDirIsOK(t *testing.T) {
	chdir(t, t.TempDir())
	if err := runValidate(nil); err != nil {
		t.Errorf("runValidate on missing routes.d: %v", err)
	}
}
