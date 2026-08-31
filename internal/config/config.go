// Package config parses Reflector's command-line flags into a Config.
package config

import (
	"flag"
	"fmt"
	"time"
)

// Config holds Reflector's command-line configuration.
type Config struct {
	Host             string
	Port             int
	HostnameOverride string
	Quiet            bool

	TCPPort        int
	TCPBanner      bool
	TCPIdleTimeout time.Duration

	RoutesDir      string
	Preset         string
	StrictBuiltins bool
	Watch          bool

	AdminPort   int
	Capture     bool
	CaptureMax  int
	NoOverrides bool

	CrtFile       string
	KeyFile       string
	TLSSelfSigned bool
	H2C           bool
}

// Default TCP idle timeout, per PLAN.md.
const defaultTCPIdleTimeout = 5 * time.Minute

// Parse parses args (typically os.Args[1:]) into a Config.
func Parse(args []string) (Config, error) {
	fs := flag.NewFlagSet("reflector", flag.ContinueOnError)

	cfg := Config{}
	fs.StringVar(&cfg.Host, "host", "0.0.0.0", "address to listen on")
	fs.IntVar(&cfg.Port, "port", 8080, "HTTP port to listen on")
	fs.StringVar(&cfg.HostnameOverride, "hostname-override", "", "override the reported hostname")
	fs.BoolVar(&cfg.Quiet, "quiet", false, "suppress request logging")

	fs.IntVar(&cfg.TCPPort, "tcp-port", 0, "enable a raw TCP echo listener on this port (0 disables it)")
	fs.BoolVar(&cfg.TCPBanner, "tcp-banner", false, "send an identity banner line on TCP connect before echoing")
	fs.DurationVar(&cfg.TCPIdleTimeout, "tcp-idle-timeout", defaultTCPIdleTimeout, "close idle TCP echo connections after this duration")

	fs.StringVar(&cfg.RoutesDir, "routes-dir", "routes.d", "directory of user-defined route YAML files")
	fs.StringVar(&cfg.Preset, "preset", "", "comma-separated preset route packs to load (rest-api,flaky,slow,auth,big-payloads)")
	fs.BoolVar(&cfg.StrictBuiltins, "strict-builtins", false, "never let user or preset routes shadow built-in routes")
	fs.BoolVar(&cfg.Watch, "watch", false, "hot-reload route files on change")

	fs.IntVar(&cfg.AdminPort, "admin-port", 8081, "port for the admin API")
	fs.BoolVar(&cfg.Capture, "capture", false, "enable the request capture ring buffer")
	fs.IntVar(&cfg.CaptureMax, "capture-max", 100, "maximum captured requests retained (FIFO eviction)")
	fs.BoolVar(&cfg.NoOverrides, "no-overrides", false, "disable per-request overrides (_status/_delay/_body/_connection/_abort)")

	fs.StringVar(&cfg.CrtFile, "crt", "", "TLS certificate file (enables HTTPS with --key)")
	fs.StringVar(&cfg.KeyFile, "key", "", "TLS private key file (enables HTTPS with --crt)")
	fs.BoolVar(&cfg.TLSSelfSigned, "tls-self-signed", false, "generate an ephemeral self-signed TLS certificate at startup")
	fs.BoolVar(&cfg.H2C, "h2c", false, "enable cleartext HTTP/2 (h2c)")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("invalid --port %d: must be between 1 and 65535", cfg.Port)
	}
	if cfg.TCPPort != 0 && (cfg.TCPPort < 1 || cfg.TCPPort > 65535) {
		return Config{}, fmt.Errorf("invalid --tcp-port %d: must be between 1 and 65535", cfg.TCPPort)
	}
	if cfg.AdminPort < 1 || cfg.AdminPort > 65535 {
		return Config{}, fmt.Errorf("invalid --admin-port %d: must be between 1 and 65535", cfg.AdminPort)
	}
	if cfg.CaptureMax < 1 {
		return Config{}, fmt.Errorf("invalid --capture-max %d: must be at least 1", cfg.CaptureMax)
	}
	if (cfg.CrtFile == "") != (cfg.KeyFile == "") {
		return Config{}, fmt.Errorf("--crt and --key must be given together")
	}
	if cfg.TLSSelfSigned && cfg.CrtFile != "" {
		return Config{}, fmt.Errorf("--tls-self-signed and --crt/--key are mutually exclusive")
	}
	if cfg.H2C && (cfg.CrtFile != "" || cfg.TLSSelfSigned) {
		return Config{}, fmt.Errorf("--h2c and TLS (--crt/--tls-self-signed) are mutually exclusive")
	}

	return cfg, nil
}
