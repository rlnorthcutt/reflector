package main

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWatchRoutesTriggersReloadOnFileChange(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, "routes.d/x.yaml", "routes:\n  - path: /x\n")

	var reloads atomic.Int32
	reload := func() (int, error) {
		reloads.Add(1)
		return 1, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- watchRoutes(ctx, "routes.d", testLogger(), reload) }()

	// Give the watcher time to start before triggering an event.
	time.Sleep(100 * time.Millisecond)
	writeFile(t, "routes.d/x.yaml", "routes:\n  - path: /x\n  - path: /y\n")

	deadline := time.Now().Add(3 * time.Second)
	for reloads.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if reloads.Load() == 0 {
		t.Fatal("expected at least one reload after editing a route file")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("watchRoutes returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watchRoutes did not return after context cancellation")
	}
}

func TestWatchRoutesIgnoresNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, "routes.d/x.yaml", "routes:\n  - path: /x\n")

	var reloads atomic.Int32
	reload := func() (int, error) {
		reloads.Add(1)
		return 1, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = watchRoutes(ctx, "routes.d", testLogger(), reload) }()

	time.Sleep(100 * time.Millisecond)
	writeFile(t, "routes.d/notes.txt", "hello")
	time.Sleep(500 * time.Millisecond)

	if reloads.Load() != 0 {
		t.Errorf("reloads = %d, want 0 for a non-route file change", reloads.Load())
	}
}

func TestWatchRoutesMissingDirDoesNotError(t *testing.T) {
	chdir(t, t.TempDir())
	err := watchRoutes(context.Background(), "routes.d", testLogger(), func() (int, error) { return 0, nil })
	if err != nil {
		t.Errorf("watchRoutes on missing dir: %v", err)
	}
}
