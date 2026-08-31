package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchDebounce coalesces bursts of filesystem events (an editor's
// save-as-temp-then-rename dance is often several events) into one
// reload.
const watchDebounce = 200 * time.Millisecond

// watchRoutes watches routesDir for route file changes, calling reload
// (debounced) on each one, until ctx is cancelled. If routesDir doesn't
// exist, it logs a warning and returns nil rather than failing startup —
// many setups run with only --preset and no routes.d at all.
func watchRoutes(ctx context.Context, routesDir string, logger *slog.Logger, reload func() (int, error)) error {
	if _, err := os.Stat(routesDir); os.IsNotExist(err) {
		logger.Warn("--watch: routes dir does not exist, nothing to watch", "dir", routesDir)
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(routesDir); err != nil {
		return fmt.Errorf("watching %s: %w", routesDir, err)
	}
	logger.Info("watching routes for changes", "dir", routesDir)

	var timer *time.Timer
	debounced := make(chan struct{}, 1)

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !isRouteFile(event.Name) {
				continue
			}
			fire := func() {
				select {
				case debounced <- struct{}{}:
				default:
				}
			}
			if timer == nil {
				timer = time.AfterFunc(watchDebounce, fire)
			} else {
				timer.Reset(watchDebounce)
			}

		case <-debounced:
			count, err := reload()
			if err != nil {
				logger.Error("route reload failed", "error", err)
				continue
			}
			logger.Info("routes reloaded (watch)", "total", count)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			logger.Error("watch error", "error", err)
		}
	}
}

func isRouteFile(name string) bool {
	switch filepath.Ext(name) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}
