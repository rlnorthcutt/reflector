// Command reflector is a single-binary HTTP echo server for load-balancer
// demos and testing. See PLAN.md for the full design.
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
	"github.com/rlnorthcutt/reflector/internal/tcpecho"
)

// shutdownGracePeriod bounds how long in-flight requests get to finish
// once shutdown starts.
const shutdownGracePeriod = 10 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "reflector:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpAddr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	srv := httpserver.New(httpAddr, id, logger, cfg.Quiet)

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
