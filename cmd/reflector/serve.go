package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/rlnorthcutt/reflector/internal/adminserver"
	"github.com/rlnorthcutt/reflector/internal/capture"
	"github.com/rlnorthcutt/reflector/internal/config"
	"github.com/rlnorthcutt/reflector/internal/health"
	"github.com/rlnorthcutt/reflector/internal/httpserver"
	"github.com/rlnorthcutt/reflector/internal/identity"
	"github.com/rlnorthcutt/reflector/internal/tcpecho"
	"github.com/rlnorthcutt/reflector/internal/tlsutil"
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

	userMux, routeCount, err := buildUserMux(cfg.RoutesDir, cfg.Preset, id)
	if err != nil {
		return fmt.Errorf("loading routes; fix them or run 'reflector validate': %w", err)
	}
	if routeCount > 0 {
		logger.Info("loaded routes", "total", routeCount)
	}
	userRoutes := httpserver.NewUserRoutes(userMux)
	healthState := health.New()

	var captureBuf *capture.Buffer
	if cfg.Capture {
		captureBuf = capture.NewBuffer(cfg.CaptureMax)
		logger.Info("request capture enabled", "max", cfg.CaptureMax)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpAddr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	srv := httpserver.New(httpAddr, id, logger, cfg.Quiet, httpserver.Options{
		UserRoutes:     userRoutes,
		StrictBuiltins: cfg.StrictBuiltins,
		Health:         healthState,
		Capture:        captureBuf,
		NoOverrides:    cfg.NoOverrides,
	})

	reload := func() (int, error) {
		mux, count, err := buildUserMux(cfg.RoutesDir, cfg.Preset, id)
		if err != nil {
			return 0, err
		}
		userRoutes.Store(mux)
		return count, nil
	}

	if cfg.TLSSelfSigned {
		cert, err := tlsutil.GenerateSelfSigned()
		if err != nil {
			return fmt.Errorf("generating self-signed certificate: %w", err)
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		logger.Info("TLS enabled", "mode", "self-signed")
	} else if cfg.CrtFile != "" {
		logger.Info("TLS enabled", "mode", "provided", "crt", cfg.CrtFile)
	}
	if cfg.H2C {
		srv.Handler = h2c.NewHandler(srv.Handler, &http2.Server{})
		logger.Info("h2c (cleartext HTTP/2) enabled")
	}

	adminAddr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.AdminPort))
	adminSrv := adminserver.New(adminAddr, &adminserver.Server{
		Health:  healthState,
		Capture: captureBuf,
		Reload:  reload,
		ListRoutes: func() []adminserver.RouteInfo {
			return currentRouteInfo(cfg.RoutesDir, cfg.Preset)
		},
	}, logger)

	errCh := make(chan error, 3)

	go func() {
		logger.Info("http server listening", "addr", httpAddr)
		var err error
		switch {
		case cfg.CrtFile != "":
			err = srv.ListenAndServeTLS(cfg.CrtFile, cfg.KeyFile)
		case cfg.TLSSelfSigned:
			err = srv.ListenAndServeTLS("", "")
		default:
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	go func() {
		logger.Info("admin server listening", "addr", adminAddr)
		if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("admin server: %w", err)
		}
	}()

	if cfg.Watch {
		go func() {
			if err := watchRoutes(ctx, cfg.RoutesDir, logger, reload); err != nil {
				errCh <- fmt.Errorf("watch: %w", err)
			}
		}()
	}

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
	if err := adminSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("admin server shutdown error", "error", err)
	}
	if tcpListener != nil {
		_ = tcpListener.Close()
	}

	return nil
}

// currentRouteInfo loads the current route table for `GET /admin/routes`,
// reflecting live disk state rather than a stale startup snapshot.
func currentRouteInfo(routesDir, presetList string) []adminserver.RouteInfo {
	rs := loadRoutes(routesDir, presetList)
	list := make([]adminserver.RouteInfo, 0, len(rs.Resolved))
	for _, r := range rs.Resolved {
		method := r.Method
		if method == "" {
			method = "ANY"
		}
		list = append(list, adminserver.RouteInfo{
			Method: method,
			Path:   r.Path,
			Status: r.Status,
			Source: r.Source,
		})
	}
	return list
}
