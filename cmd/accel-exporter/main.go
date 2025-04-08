package main

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/taihen/accel-exporter/pkg/collector"
	"github.com/taihen/accel-exporter/pkg/config"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cfg := config.NewConfig()

	log.Printf("Starting accel-exporter version %s (%s) built on %s", version, commit, date)
	log.Printf("Listening on %s, metrics path: %s", cfg.ListenAddress, cfg.MetricsPath)

	// Create and register collector
	accelCollector := collector.NewAccelCollector(cfg.AccelCmdPath)
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
		w.Write([]byte(`<html>
			<head><title>Accel-PPP Exporter</title></head>
			<body>
				<h1>Accel-PPP Exporter</h1>
				<p><a href="` + cfg.MetricsPath + `">Metrics</a></p>
			</body>
		</html>`))
	})

	log.Fatal(http.ListenAndServe(cfg.ListenAddress, nil))
}
