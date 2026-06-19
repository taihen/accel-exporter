// Command accel-exporter is a Prometheus exporter for Accel-PPP, exposing the
// metrics reported by `accel-cmd show stat`.
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

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
	accelCollector := collector.NewAccelCollector(cfg.AccelCmdPath, cfg.ScrapeTimeout)
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

	// Set up HTTP server with an explicit mux and timeouts. ReadHeaderTimeout
	// guards against Slowloris-style header dribbling; WriteTimeout is kept
	// comfortably above the scrape timeout so a legitimately slow scrape is
	// never truncated.
	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html>
			<head><title>Accel-PPP Exporter</title></head>
			<body>
				<h1>Accel-PPP Exporter</h1>
				<p><a href="%s">Metrics</a></p>
				<p><small>%s</small></p>
			</body>
		</html>`, cfg.MetricsPath, versionInfo())
	})

	// Mirror the collector's clamp so a non-positive -accel-cmd.timeout cannot
	// produce a too-short WriteTimeout that would truncate a legitimate scrape.
	scrapeTimeout := cfg.ScrapeTimeout
	if scrapeTimeout <= 0 {
		scrapeTimeout = collector.DefaultScrapeTimeout
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      scrapeTimeout + 10*time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	log.Fatal(srv.ListenAndServe())
}
