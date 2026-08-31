package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rlnorthcutt/reflector/internal/config"
	"github.com/rlnorthcutt/reflector/internal/httpserver"
	"github.com/rlnorthcutt/reflector/internal/identity"
	"github.com/rlnorthcutt/reflector/internal/routes"
	"github.com/rlnorthcutt/reflector/internal/tcpecho"
)

// shutdownGracePeriod bounds how long in-flight requests get to finish
// once shutdown starts.
const shutdownGracePeriod = 10 * time.Second

func runServe(args []string) error {
	cfg, err := config.Parse(args)
	if err != nil {
		return err
	}

	logLevel := slog.LevelInfo
	if cfg.Quiet {
		logLevel = slog.LevelWarn
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	id := identity.New(cfg.HostnameOverride, cfg.Port)
	logger.Info("starting reflector",
		"hostname", id.Hostname,
		"instance_id", id.InstanceID,
		"port", id.Port,
		"version", id.Version,
	)

	var userMux *http.ServeMux
	rs := loadRoutes(cfg.RoutesDir, cfg.Preset)
	if len(rs.Errors) > 0 {
		for _, e := range rs.Errors {
			fmt.Fprintln(os.Stderr, "reflector: route error:", e)
		}
		return fmt.Errorf("%d route error(s); fix them or run 'reflector validate'", len(rs.Errors))
	}
	if len(rs.Resolved) > 0 {
		userMux, err = routes.BuildMux(rs.Resolved, id)
		if err != nil {
			return fmt.Errorf("building route table: %w", err)
		}
		logger.Info("loaded routes", "user", len(rs.User), "preset", len(rs.Presets), "total", len(rs.Resolved))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpAddr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	srv := httpserver.New(httpAddr, id, logger, cfg.Quiet, httpserver.Options{
		UserMux:        userMux,
		StrictBuiltins: cfg.StrictBuiltins,
	})

	errCh := make(chan error, 2)

	go func() {
		logger.Info("http server listening", "addr", httpAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	var tcpListener net.Listener
	if cfg.TCPPort != 0 {
		tcpAddr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.TCPPort))
		tcpListener, err = net.Listen("tcp", tcpAddr)
		if err != nil {
			return fmt.Errorf("tcp echo listener: %w", err)
		}
		echo := &tcpecho.Listener{
			Banner:      cfg.TCPBanner,
			IdleTimeout: cfg.TCPIdleTimeout,
			Logger:      logger,
		}
		logger.Info("tcp echo listening", "addr", tcpAddr, "banner", cfg.TCPBanner)
		go func() {
			if err := echo.Serve(ctx, tcpListener, id, cfg.TCPPort); err != nil {
				errCh <- fmt.Errorf("tcp echo listener: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-errCh:
		logger.Error("server error", "error", err)
		stop()
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err)
	}
	if tcpListener != nil {
		_ = tcpListener.Close()
	}

	return nil
}
