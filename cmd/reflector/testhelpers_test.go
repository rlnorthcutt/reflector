package main

import (
	"os"
	"testing"
)

// chdir switches the working directory to dir for the duration of the
// test, restoring it on cleanup. Equivalent to testing.T.Chdir, which
// requires Go 1.24; this module targets Go 1.22.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}
