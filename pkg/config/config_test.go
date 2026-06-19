package config

import (
	"flag"
	"os"
	"testing"
)

// withArgs runs fn with os.Args replaced and the global flag set reset, so each
// case calls NewConfig() (which uses the package-level flag.CommandLine) from a
// clean state. State is restored on return.
func withArgs(t *testing.T, args []string, fn func()) {
	t.Helper()
	origArgs := os.Args
	origFlags := flag.CommandLine
	t.Cleanup(func() {
		os.Args = origArgs
		flag.CommandLine = origFlags
	})
	os.Args = append([]string{"accel-exporter"}, args...)
	flag.CommandLine = flag.NewFlagSet("accel-exporter", flag.ContinueOnError)
	fn()
}

func TestNewConfigDefaults(t *testing.T) {
	t.Setenv("ACCEL_EXPORTER_PORT", "")
	withArgs(t, nil, func() {
		cfg := NewConfig()
		if cfg.ListenAddress != ":9101" {
			t.Errorf("ListenAddress = %q, want :9101", cfg.ListenAddress)
		}
		if cfg.MetricsPath != "/metrics" {
			t.Errorf("MetricsPath = %q, want /metrics", cfg.MetricsPath)
		}
		if cfg.AccelCmdPath != "accel-cmd" {
			t.Errorf("AccelCmdPath = %q, want accel-cmd", cfg.AccelCmdPath)
		}
		if cfg.LogLevel != "info" {
			t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
		}
	})
}

func TestNewConfigFlags(t *testing.T) {
	t.Setenv("ACCEL_EXPORTER_PORT", "")
	args := []string{
		"-web.listen-address=:9999",
		"-web.metrics-path=/m",
		"-accel-cmd.path=/usr/sbin/accel-cmd",
		"-log.level=debug",
	}
	withArgs(t, args, func() {
		cfg := NewConfig()
		if cfg.ListenAddress != ":9999" {
			t.Errorf("ListenAddress = %q, want :9999", cfg.ListenAddress)
		}
		if cfg.MetricsPath != "/m" {
			t.Errorf("MetricsPath = %q, want /m", cfg.MetricsPath)
		}
		if cfg.AccelCmdPath != "/usr/sbin/accel-cmd" {
			t.Errorf("AccelCmdPath = %q, want /usr/sbin/accel-cmd", cfg.AccelCmdPath)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
		}
	})
}

// TestNewConfigPortEnvOverride verifies ACCEL_EXPORTER_PORT wins over the
// listen-address flag default (and any flag value).
func TestNewConfigPortEnvOverride(t *testing.T) {
	t.Setenv("ACCEL_EXPORTER_PORT", "8080")
	withArgs(t, []string{"-web.listen-address=:9999"}, func() {
		cfg := NewConfig()
		if cfg.ListenAddress != ":8080" {
			t.Errorf("ListenAddress = %q, want :8080 (env override)", cfg.ListenAddress)
		}
	})
}
