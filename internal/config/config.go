// Package config parses Reflector's command-line flags into a Config.
package config

import (
	"flag"
	"fmt"
	"time"
)

// Config holds all v0.1 command-line configuration.
type Config struct {
	Host             string
	Port             int
	HostnameOverride string
	Quiet            bool

	TCPPort        int
	TCPBanner      bool
	TCPIdleTimeout time.Duration
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

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("invalid --port %d: must be between 1 and 65535", cfg.Port)
	}
	if cfg.TCPPort != 0 && (cfg.TCPPort < 1 || cfg.TCPPort > 65535) {
		return Config{}, fmt.Errorf("invalid --tcp-port %d: must be between 1 and 65535", cfg.TCPPort)
	}

	return cfg, nil
}
