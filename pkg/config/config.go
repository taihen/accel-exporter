// Package config resolves the exporter's runtime configuration from command
// line flags and environment variables.
package config

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// Config holds the exporter configuration
type Config struct {
	ListenAddress string
	MetricsPath   string
	AccelCmdPath  string
	LogLevel      string
	ScrapeTimeout time.Duration
}

// NewConfig creates a new configuration from command line flags
func NewConfig() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.ListenAddress, "web.listen-address", ":9101", "Address to listen on for web interface and telemetry")
	flag.StringVar(&cfg.MetricsPath, "web.metrics-path", "/metrics", "Path under which to expose metrics")
	flag.StringVar(&cfg.AccelCmdPath, "accel-cmd.path", "accel-cmd", "Path to accel-cmd binary")
	flag.StringVar(&cfg.LogLevel, "log.level", "info", "Log level (debug, info, warn, error)")
	flag.DurationVar(&cfg.ScrapeTimeout, "accel-cmd.timeout", 5*time.Second, "Maximum time to wait for accel-cmd to return")

	flag.Parse()

	// Also check environment variables
	if envPort := os.Getenv("ACCEL_EXPORTER_PORT"); envPort != "" {
		cfg.ListenAddress = fmt.Sprintf(":%s", envPort)
	}

	return cfg
}
