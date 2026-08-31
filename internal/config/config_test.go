package config

import (
	"testing"
	"time"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.TCPPort != 0 {
		t.Errorf("TCPPort = %d, want 0 (disabled)", cfg.TCPPort)
	}
	if cfg.TCPIdleTimeout != defaultTCPIdleTimeout {
		t.Errorf("TCPIdleTimeout = %v, want %v", cfg.TCPIdleTimeout, defaultTCPIdleTimeout)
	}
	if cfg.RoutesDir != "routes.d" {
		t.Errorf("RoutesDir = %q, want routes.d", cfg.RoutesDir)
	}
	if cfg.Preset != "" {
		t.Errorf("Preset = %q, want empty", cfg.Preset)
	}
	if cfg.StrictBuiltins {
		t.Error("StrictBuiltins = true, want false")
	}
}

func TestParseOverrides(t *testing.T) {
	cfg, err := Parse([]string{
		"--port", "9090",
		"--host", "127.0.0.1",
		"--hostname-override", "demo-1",
		"--quiet",
		"--tcp-port", "9091",
		"--tcp-banner",
		"--tcp-idle-timeout", "30s",
		"--routes-dir", "custom-routes",
		"--preset", "rest-api,flaky",
		"--strict-builtins",
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Port != 9090 || cfg.Host != "127.0.0.1" || cfg.HostnameOverride != "demo-1" || !cfg.Quiet {
		t.Errorf("cfg = %+v, unexpected values", cfg)
	}
	if cfg.TCPPort != 9091 || !cfg.TCPBanner || cfg.TCPIdleTimeout != 30*time.Second {
		t.Errorf("cfg tcp fields = %+v, unexpected values", cfg)
	}
	if cfg.RoutesDir != "custom-routes" || cfg.Preset != "rest-api,flaky" || !cfg.StrictBuiltins {
		t.Errorf("cfg route fields = %+v, unexpected values", cfg)
	}
}

func TestParseRejectsInvalidPort(t *testing.T) {
	if _, err := Parse([]string{"--port", "0"}); err == nil {
		t.Error("expected error for --port 0")
	}
	if _, err := Parse([]string{"--port", "70000"}); err == nil {
		t.Error("expected error for --port 70000")
	}
}

func TestParseRejectsInvalidTCPPort(t *testing.T) {
	if _, err := Parse([]string{"--tcp-port", "-1"}); err == nil {
		t.Error("expected error for --tcp-port -1")
	}
}
