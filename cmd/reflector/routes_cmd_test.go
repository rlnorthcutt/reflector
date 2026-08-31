package main

import "testing"

func TestRunRoutesCmdSucceeds(t *testing.T) {
	chdir(t, t.TempDir())
	writeFile(t, "routes.d/x.yaml", "routes:\n  - path: /x\n    response:\n      body: hi\n")

	if err := runRoutesCmd([]string{"--preset", "flaky"}); err != nil {
		t.Errorf("runRoutesCmd: %v", err)
	}
}

func TestRunRoutesCmdFailsOnUnknownPreset(t *testing.T) {
	chdir(t, t.TempDir())
	if err := runRoutesCmd([]string{"--preset", "nope"}); err == nil {
		t.Error("runRoutesCmd: expected error for unknown preset")
	}
}
