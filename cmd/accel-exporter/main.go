package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/taihen/accel-exporter/pkg/collector"
	"github.com/taihen/accel-exporter/pkg/config"
)

// Version information set by build flags
var (
	version = "dev"     // Semantic version from git tag
	commit  = "unknown" // Git commit hash
	date    = "unknown" // Build timestamp
)

// versionInfo returns a formatted string with build information
func versionInfo() string {
	return fmt.Sprintf("accel-exporter version %s (%s) built at %s", version, commit, date)
}

func main() {
	cfg := config.NewConfig()

	log.Printf("Starting %s", versionInfo())
	log.Printf("Listening on %s, metrics path: %s", cfg.ListenAddress, cfg.MetricsPath)

	// Create and register collector
	accelCollector := collector.NewAccelCollector(cfg.AccelCmdPath, cfg.AccelCmdPwd)
	prometheus.MustRegister(accelCollector)

	// Add version information
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "accel_exporter_build_info",
			Help: "A metric with a constant '1' value labeled by version, commit, and date of build.",
		},
		[]string{"version", "commit", "date"},
	)
	buildInfo.WithLabelValues(version, commit, date).Set(1)
	prometheus.MustRegister(buildInfo)

	// Set up HTTP server
	http.Handle(cfg.MetricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(fmt.Sprintf(`<html>
			<head><title>Accel-PPP Exporter</title></head>
			<body>
				<h1>Accel-PPP Exporter</h1>
				<p><a href="%s">Metrics</a></p>
				<p><small>%s</small></p>
			</body>
		</html>`, cfg.MetricsPath, versionInfo())))
	})

	log.Fatal(http.ListenAndServe(cfg.ListenAddress, nil))
}
