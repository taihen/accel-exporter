package collector

import (
	"context"
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

// sessionLabels identify a session: its sid, plus the username and realm to
// group by. The same username can have several live sessions (accel-ppp allows
// that unless single-session is set), so the username alone is not unique. A
// new session gets a new sid, so a reconnect starts a new series; use
// "sum by (username)" to follow a subscriber across reconnects. Deliberately no
// interface or IP label: interface numbers are reused by unrelated sessions
// and addresses change on every reconnect.
var sessionLabels = []string{"sid", "username", "realm"}

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
)

var sessionDescs = []*prometheus.Desc{
	sessionRxBytesDesc, sessionTxBytesDesc, sessionRxPacketsDesc, sessionTxPacketsDesc,
	sessionUptimeDesc, sessionRxRateLimitDesc, sessionTxRateLimitDesc, sessionParseErrorsDesc,
}

// collectSessions runs "accel-cmd show sessions" and emits the per-session
// metrics. Like collectSwitchShow, a failure is logged and only drops these
// series for the scrape; it never affects accel_up or the rest of Collect.
func (c *AccelCollector) collectSessions(ctx context.Context, ch chan<- prometheus.Metric) {
	sessions, parseErrors, err := parser.CollectSessions(ctx, c.accelCmdPath)
	if err != nil {
		log.Printf("show sessions unavailable, per-session metrics dropped for this scrape: %v", err)
		return
	}

	for _, s := range sessions {
		realm := parser.RealmOf(s.Username)
		ch <- prometheus.MustNewConstMetric(sessionRxBytesDesc, prometheus.CounterValue, s.RxBytes, s.SID, s.Username, realm)
		ch <- prometheus.MustNewConstMetric(sessionTxBytesDesc, prometheus.CounterValue, s.TxBytes, s.SID, s.Username, realm)
		ch <- prometheus.MustNewConstMetric(sessionRxPacketsDesc, prometheus.CounterValue, s.RxPackets, s.SID, s.Username, realm)
		ch <- prometheus.MustNewConstMetric(sessionTxPacketsDesc, prometheus.CounterValue, s.TxPackets, s.SID, s.Username, realm)
		ch <- prometheus.MustNewConstMetric(sessionUptimeDesc, prometheus.GaugeValue, s.Uptime, s.SID, s.Username, realm)
		if s.HasRate {
			ch <- prometheus.MustNewConstMetric(sessionTxRateLimitDesc, prometheus.GaugeValue, kbitToBytesPerSecond(s.RateDownKbit), s.SID, s.Username, realm)
			ch <- prometheus.MustNewConstMetric(sessionRxRateLimitDesc, prometheus.GaugeValue, kbitToBytesPerSecond(s.RateUpKbit), s.SID, s.Username, realm)
		}
	}

	ch <- prometheus.MustNewConstMetric(sessionParseErrorsDesc, prometheus.GaugeValue, float64(parseErrors))
}

// kbitToBytesPerSecond converts accel-ppp's Kbit/s (1 Kbit = 1000 bit) to bytes/s.
func kbitToBytesPerSecond(kbit float64) float64 {
	return kbit * 1000 / 8
}
