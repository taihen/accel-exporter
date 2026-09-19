package collector

import (
	"log"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/taihen/accel-exporter/pkg/parser"
)

// WithSessions enables the optional per-session metrics, read from
// "accel-cmd show sessions". Off by default: it adds one series set per live
// session, which is fine for a small fleet but a real cost at thousands of
// sessions.
func WithSessions() Option {
	return func(c *AccelCollector) { c.sessions = true }
}

// sessionLabels identify a subscriber. Deliberately only the username (stable
// across reconnects) and its realm: the ppp interface number is reused by
// unrelated sessions and IP addresses change on every reconnect, so neither
// belongs on a per-subscriber series.
var sessionLabels = []string{"username", "realm"}

var (
	sessionRxBytesDesc   = newDesc("accel_session_rx_bytes_total", "Bytes received from the subscriber in the current session (restarts at zero on reconnect).", sessionLabels...)
	sessionTxBytesDesc   = newDesc("accel_session_tx_bytes_total", "Bytes sent to the subscriber in the current session (restarts at zero on reconnect).", sessionLabels...)
	sessionRxPacketsDesc = newDesc("accel_session_rx_packets_total", "Packets received from the subscriber in the current session (restarts at zero on reconnect).", sessionLabels...)
	sessionTxPacketsDesc = newDesc("accel_session_tx_packets_total", "Packets sent to the subscriber in the current session (restarts at zero on reconnect).", sessionLabels...)
	sessionUptimeDesc    = newDesc("accel_session_uptime_seconds", "Seconds since the subscriber's current session started.", sessionLabels...)
	// Shaper limits follow the rx_/tx_ naming of the byte counters above and,
	// like every other unit in this collector, are exported in base units
	// (bytes per second; accel-ppp reports Kbit/s). tx = towards the
	// subscriber ("down" in accel-ppp), rx = from the subscriber ("up").
	sessionRxRateLimitDesc = newDesc("accel_session_rx_rate_limit_bytes_per_second", "Shaper limit for traffic received from the subscriber, in bytes per second. Absent for unshaped sessions.", sessionLabels...)
	sessionTxRateLimitDesc = newDesc("accel_session_tx_rate_limit_bytes_per_second", "Shaper limit for traffic sent to the subscriber, in bytes per second. Absent for unshaped sessions.", sessionLabels...)

	sessionParseErrorsDesc = newDesc("accel_session_parse_errors", "Lines of 'accel-cmd show sessions' output that could not be parsed in the last scrape.")
	sessionDuplicatesDesc  = newDesc("accel_session_duplicates_dropped", "Sessions dropped in the last scrape because the same username appeared twice (the newest is kept).")
)

var sessionDescs = []*prometheus.Desc{
	sessionRxBytesDesc, sessionTxBytesDesc, sessionRxPacketsDesc, sessionTxPacketsDesc,
	sessionUptimeDesc, sessionRxRateLimitDesc, sessionTxRateLimitDesc, sessionParseErrorsDesc, sessionDuplicatesDesc,
}

// collectSessions runs "accel-cmd show sessions" and emits the per-session
// metrics. Like collectSwitchShow, a failure is logged and only drops these
// series for the scrape; it never affects accel_up or the rest of Collect.
func (c *AccelCollector) collectSessions(ch chan<- prometheus.Metric) {
	sessions, parseErrors, err := parser.CollectSessions(c.accelCmdPath, c.timeout)
	if err != nil {
		log.Printf("show sessions unavailable, per-session metrics dropped for this scrape: %v", err)
		return
	}

	sessions, duplicates := parser.DedupeSessions(sessions)

	for _, s := range sessions {
		realm := parser.RealmOf(s.Username)
		ch <- prometheus.MustNewConstMetric(sessionRxBytesDesc, prometheus.CounterValue, s.RxBytes, s.Username, realm)
		ch <- prometheus.MustNewConstMetric(sessionTxBytesDesc, prometheus.CounterValue, s.TxBytes, s.Username, realm)
		ch <- prometheus.MustNewConstMetric(sessionRxPacketsDesc, prometheus.CounterValue, s.RxPackets, s.Username, realm)
		ch <- prometheus.MustNewConstMetric(sessionTxPacketsDesc, prometheus.CounterValue, s.TxPackets, s.Username, realm)
		ch <- prometheus.MustNewConstMetric(sessionUptimeDesc, prometheus.GaugeValue, s.Uptime, s.Username, realm)
		if s.HasRate {
			ch <- prometheus.MustNewConstMetric(sessionTxRateLimitDesc, prometheus.GaugeValue, kbitToBytesPerSecond(s.RateDownKbit), s.Username, realm)
			ch <- prometheus.MustNewConstMetric(sessionRxRateLimitDesc, prometheus.GaugeValue, kbitToBytesPerSecond(s.RateUpKbit), s.Username, realm)
		}
	}

	ch <- prometheus.MustNewConstMetric(sessionParseErrorsDesc, prometheus.GaugeValue, float64(parseErrors))
	ch <- prometheus.MustNewConstMetric(sessionDuplicatesDesc, prometheus.GaugeValue, float64(duplicates))
}

// kbitToBytesPerSecond converts accel-ppp's Kbit/s (1 Kbit = 1000 bit) to bytes/s.
func kbitToBytesPerSecond(kbit float64) float64 {
	return kbit * 1000 / 8
}
