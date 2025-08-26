package config

import (
	"flag"
	"os"
	"strconv"
)

// Config holds the exporter configuration
type Config struct {
	ListenAddress string
	MetricsPath   string
	AccelCmdPath  string
	AccelCmdPwd   string
	AccelHost     string
	AccelPort     int
	LogLevel      string
}

// NewConfig creates a new configuration from command line flags
func NewConfig() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.ListenAddress, "web.listen-address", ":9101", "Address to listen on for web interface and telemetry")
	flag.StringVar(&cfg.MetricsPath, "web.metrics-path", "/metrics", "Path under which to expose metrics")
	flag.StringVar(&cfg.AccelCmdPath, "accel-cmd.path", "accel-cmd", "Path to accel-cmd binary")
	flag.StringVar(&cfg.AccelCmdPwd, "accel-cmd.pwd", "", "Password to connect to accel-cmd")
	flag.StringVar(&cfg.AccelHost, "accel-telnet.host", "", "Host to connect (preferred over accel-cmd.path)")
	flag.IntVar(&cfg.AccelPort, "accel-telnet.port", 0, "Port to connect (preferred over accel-cmd.path)")
	flag.StringVar(&cfg.LogLevel, "log.level", "info", "Log level (debug, info, warn, error)")

	flag.Parse()

	// Also check environment variables
	if envListenAddr := os.Getenv("ACCEL_EXPORTER_LISTEN_ADDR"); envListenAddr != "" {
		cfg.ListenAddress = envListenAddr
	}

	if envMetricsPath := os.Getenv("ACCEL_EXPORTER_METRICS_PATH"); envMetricsPath != "" {
		cfg.MetricsPath = envMetricsPath
	}

	if envPath := os.Getenv("ACCEL_EXPORTER_PATH"); envPath != "" {
		cfg.AccelCmdPath = envPath
	}

	if envPwd := os.Getenv("ACCEL_EXPORTER_PASSWORD"); envPwd != "" {
		cfg.AccelCmdPwd = envPwd
	}

	if envHost := os.Getenv("ACCEL_EXPORTER_HOST"); envHost != "" {
		cfg.AccelHost = envHost
	}

	if envPort := os.Getenv("ACCEL_EXPORTER_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			cfg.AccelPort = p
		}
	}
	if envLogLevel := os.Getenv("ACCEL_EXPORTER_LOG_LEVEL"); envLogLevel != "" {
		cfg.LogLevel = envLogLevel
	}

	return cfg
}
