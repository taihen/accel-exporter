// Package collector implements the prometheus.Collector that scrapes accel-ppp
// statistics and exposes them as Prometheus metrics.
//
// The collector is stateless: Collect parses a fresh snapshot and emits const
// metrics built on the fly, so concurrent scrapes (e.g. an HA Prometheus pair)
// never share mutable metric state. The only persistent metric is the
// cumulative scrape-failure counter, whose increments are atomic.
package collector

import (
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/taihen/accel-exporter/pkg/parser"
)

// radiusLabels are the labels attached to every per-RADIUS-server metric.
var radiusLabels = []string{"server_id", "server_ip"}

func newDesc(name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc(name, help, labels, nil)
}

// Descriptors are built once and shared across scrapes (they are immutable).
var (
	upDesc      = newDesc("accel_up", "Was the last accel-cmd scrape successful.")
	uptimeDesc  = newDesc("accel_uptime_seconds", "Uptime of accel-ppp in seconds.")
	cpuDesc     = newDesc("accel_cpu_usage_percent", "CPU usage percentage.")
	memRSSDesc  = newDesc("accel_memory_rss_bytes", "RSS memory usage in bytes.")
	memVirtDesc = newDesc("accel_memory_virtual_bytes", "Virtual memory usage in bytes.")

	coreMempoolAllocatedDesc = newDesc("accel_core_mempool_allocated_bytes", "Allocated memory pool size.")
	coreMempoolAvailableDesc = newDesc("accel_core_mempool_available_bytes", "Available memory pool size.")
	coreThreadCountDesc      = newDesc("accel_core_thread_count", "Number of core threads.")
	coreThreadActiveDesc     = newDesc("accel_core_thread_active", "Number of active core threads.")
	coreContextCountDesc     = newDesc("accel_core_context_count", "Number of core contexts.")
	coreContextSleepingDesc  = newDesc("accel_core_context_sleeping", "Number of sleeping core contexts.")
	coreContextPendingDesc   = newDesc("accel_core_context_pending", "Number of pending core contexts.")
	coreMDHandlerCountDesc   = newDesc("accel_core_md_handler_count", "Number of MD handlers.")
	coreMDHandlerPendingDesc = newDesc("accel_core_md_handler_pending", "Number of pending MD handlers.")
	coreTimerCountDesc       = newDesc("accel_core_timer_count", "Number of core timers.")
	coreTimerPendingDesc     = newDesc("accel_core_timer_pending", "Number of pending core timers.")

	sessionsStartingDesc  = newDesc("accel_sessions_starting", "Number of sessions starting.")
	sessionsActiveDesc    = newDesc("accel_sessions_active", "Number of active sessions.")
	sessionsFinishingDesc = newDesc("accel_sessions_finishing", "Number of sessions finishing.")

	pppoeStartingDesc    = newDesc("accel_pppoe_starting", "Number of PPPoE sessions starting.")
	pppoeActiveDesc      = newDesc("accel_pppoe_active", "Number of active PPPoE sessions.")
	pppoeDelayedPADODesc = newDesc("accel_pppoe_delayed_pado_total", "Total delayed PADO packets.")
	pppoeRecvPADIDesc    = newDesc("accel_pppoe_recv_padi_total", "Total received PADI packets.")
	pppoeDropPADIDesc    = newDesc("accel_pppoe_drop_padi_total", "Total dropped PADI packets.")
	pppoeSentPADODesc    = newDesc("accel_pppoe_sent_pado_total", "Total sent PADO packets.")
	pppoeRecvPADRDesc    = newDesc("accel_pppoe_recv_padr_total", "Total received PADR packets.")
	pppoeRecvPADRDupDesc = newDesc("accel_pppoe_recv_padr_dup_total", "Total received duplicate PADR packets.")
	pppoeSentPADSDesc    = newDesc("accel_pppoe_sent_pads_total", "Total sent PADS packets.")
	pppoeFilteredDesc    = newDesc("accel_pppoe_filtered_total", "Total filtered PPPoE packets.")

	radiusStateDesc            = newDesc("accel_radius_state", "State of RADIUS server (1 = active, 0 = inactive).", radiusLabels...)
	radiusFailCountDesc        = newDesc("accel_radius_fail_count_total", "Total RADIUS server fail count.", radiusLabels...)
	radiusRequestCountDesc     = newDesc("accel_radius_request_count", "Current RADIUS server request count.", radiusLabels...)
	radiusQueueLengthDesc      = newDesc("accel_radius_queue_length", "Current RADIUS server queue length.", radiusLabels...)
	radiusAuthSentDesc         = newDesc("accel_radius_auth_sent_total", "Total RADIUS auth packets sent.", radiusLabels...)
	radiusAuthLostTotalDesc    = newDesc("accel_radius_auth_lost_total", "Total RADIUS auth packets lost.", radiusLabels...)
	radiusAuthLost5mDesc       = newDesc("accel_radius_auth_lost_5m", "RADIUS auth packets lost in the last 5 minutes.", radiusLabels...)
	radiusAuthLost1mDesc       = newDesc("accel_radius_auth_lost_1m", "RADIUS auth packets lost in the last 1 minute.", radiusLabels...)
	radiusAuthAvgTime5mDesc    = newDesc("accel_radius_auth_avg_time_5m_seconds", "Average RADIUS auth response time in the last 5 minutes (seconds).", radiusLabels...)
	radiusAuthAvgTime1mDesc    = newDesc("accel_radius_auth_avg_time_1m_seconds", "Average RADIUS auth response time in the last 1 minute (seconds).", radiusLabels...)
	radiusAcctSentDesc         = newDesc("accel_radius_acct_sent_total", "Total RADIUS accounting packets sent.", radiusLabels...)
	radiusAcctLostTotalDesc    = newDesc("accel_radius_acct_lost_total", "Total RADIUS accounting packets lost.", radiusLabels...)
	radiusAcctLost5mDesc       = newDesc("accel_radius_acct_lost_5m", "RADIUS accounting packets lost in the last 5 minutes.", radiusLabels...)
	radiusAcctLost1mDesc       = newDesc("accel_radius_acct_lost_1m", "RADIUS accounting packets lost in the last 1 minute.", radiusLabels...)
	radiusAcctAvgTime5mDesc    = newDesc("accel_radius_acct_avg_time_5m_seconds", "Average RADIUS accounting response time in the last 5 minutes (seconds).", radiusLabels...)
	radiusAcctAvgTime1mDesc    = newDesc("accel_radius_acct_avg_time_1m_seconds", "Average RADIUS accounting response time in the last 1 minute (seconds).", radiusLabels...)
	radiusInterimSentDesc      = newDesc("accel_radius_interim_sent_total", "Total RADIUS interim accounting packets sent.", radiusLabels...)
	radiusInterimLostTotalDesc = newDesc("accel_radius_interim_lost_total", "Total RADIUS interim accounting packets lost.", radiusLabels...)
	radiusInterimLost5mDesc    = newDesc("accel_radius_interim_lost_5m", "RADIUS interim accounting packets lost in the last 5 minutes.", radiusLabels...)
	radiusInterimLost1mDesc    = newDesc("accel_radius_interim_lost_1m", "RADIUS interim accounting packets lost in the last 1 minute.", radiusLabels...)
	radiusInterimAvgTime5mDesc = newDesc("accel_radius_interim_avg_time_5m_seconds", "Average RADIUS interim accounting response time in the last 5 minutes (seconds).", radiusLabels...)
	radiusInterimAvgTime1mDesc = newDesc("accel_radius_interim_avg_time_1m_seconds", "Average RADIUS interim accounting response time in the last 1 minute (seconds).", radiusLabels...)
)

// allDescs lists every descriptor the collector can emit, for Describe.
var allDescs = []*prometheus.Desc{
	upDesc, uptimeDesc, cpuDesc, memRSSDesc, memVirtDesc,
	coreMempoolAllocatedDesc, coreMempoolAvailableDesc, coreThreadCountDesc, coreThreadActiveDesc,
	coreContextCountDesc, coreContextSleepingDesc, coreContextPendingDesc,
	coreMDHandlerCountDesc, coreMDHandlerPendingDesc, coreTimerCountDesc, coreTimerPendingDesc,
	sessionsStartingDesc, sessionsActiveDesc, sessionsFinishingDesc,
	pppoeStartingDesc, pppoeActiveDesc, pppoeDelayedPADODesc, pppoeRecvPADIDesc, pppoeDropPADIDesc,
	pppoeSentPADODesc, pppoeRecvPADRDesc, pppoeRecvPADRDupDesc, pppoeSentPADSDesc, pppoeFilteredDesc,
	radiusStateDesc, radiusFailCountDesc, radiusRequestCountDesc, radiusQueueLengthDesc,
	radiusAuthSentDesc, radiusAuthLostTotalDesc, radiusAuthLost5mDesc, radiusAuthLost1mDesc,
	radiusAuthAvgTime5mDesc, radiusAuthAvgTime1mDesc,
	radiusAcctSentDesc, radiusAcctLostTotalDesc, radiusAcctLost5mDesc, radiusAcctLost1mDesc,
	radiusAcctAvgTime5mDesc, radiusAcctAvgTime1mDesc,
	radiusInterimSentDesc, radiusInterimLostTotalDesc, radiusInterimLost5mDesc, radiusInterimLost1mDesc,
	radiusInterimAvgTime5mDesc, radiusInterimAvgTime1mDesc,
}

// DefaultScrapeTimeout bounds an accel-cmd invocation when none is configured.
const DefaultScrapeTimeout = 5 * time.Second

// AccelCollector implements the prometheus.Collector interface
type AccelCollector struct {
	accelCmdPath string
	timeout      time.Duration

	// scrapeFailures is the only persistent metric: a cumulative counter whose
	// Inc is atomic and safe under concurrent scrapes.
	scrapeFailures prometheus.Counter
}

// NewAccelCollector creates a new AccelCollector. A non-positive timeout falls
// back to DefaultScrapeTimeout.
func NewAccelCollector(accelCmdPath string, timeout time.Duration) *AccelCollector {
	if timeout <= 0 {
		timeout = DefaultScrapeTimeout
	}
	return &AccelCollector{
		accelCmdPath: accelCmdPath,
		timeout:      timeout,
		scrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_scrape_failures_total",
			Help: "Number of errors while scraping accel-cmd.",
		}),
	}
}

// Describe implements the prometheus.Collector interface
func (c *AccelCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range allDescs {
		ch <- d
	}
	c.scrapeFailures.Describe(ch)
}

// Collect implements the prometheus.Collector interface. It builds const
// metrics from a fresh snapshot, so it holds no mutable state between or during
// scrapes and is safe to run concurrently.
func (c *AccelCollector) Collect(ch chan<- prometheus.Metric) {
	stats, err := parser.CollectStats(c.accelCmdPath, c.timeout)
	if err != nil {
		c.scrapeFailures.Inc()
		ch <- c.scrapeFailures
		ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 0)
		log.Printf("Error collecting stats: %v", err)
		return
	}

	ch <- c.scrapeFailures
	ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 1)

	gauge := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v)
	}
	counter := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}

	// General
	gauge(uptimeDesc, stats.Uptime)
	gauge(cpuDesc, stats.CPUPercent)
	gauge(memRSSDesc, stats.MemRSS*1024)   // KB -> bytes
	gauge(memVirtDesc, stats.MemVirt*1024) // KB -> bytes

	// Core
	gauge(coreMempoolAllocatedDesc, stats.Core.MempoolAllocated)
	gauge(coreMempoolAvailableDesc, stats.Core.MempoolAvailable)
	gauge(coreThreadCountDesc, stats.Core.ThreadCount)
	gauge(coreThreadActiveDesc, stats.Core.ThreadActive)
	gauge(coreContextCountDesc, stats.Core.ContextCount)
	gauge(coreContextSleepingDesc, stats.Core.ContextSleeping)
	gauge(coreContextPendingDesc, stats.Core.ContextPending)
	gauge(coreMDHandlerCountDesc, stats.Core.MDHandlerCount)
	gauge(coreMDHandlerPendingDesc, stats.Core.MDHandlerPending)
	gauge(coreTimerCountDesc, stats.Core.TimerCount)
	gauge(coreTimerPendingDesc, stats.Core.TimerPending)

	// Sessions
	gauge(sessionsStartingDesc, stats.Sessions.Starting)
	gauge(sessionsActiveDesc, stats.Sessions.Active)
	gauge(sessionsFinishingDesc, stats.Sessions.Finishing)

	// PPPoE
	gauge(pppoeStartingDesc, stats.PPPoE.Starting)
	gauge(pppoeActiveDesc, stats.PPPoE.Active)
	counter(pppoeDelayedPADODesc, stats.PPPoE.DelayedPADO)
	counter(pppoeRecvPADIDesc, stats.PPPoE.RecvPADI)
	counter(pppoeDropPADIDesc, stats.PPPoE.DropPADI)
	counter(pppoeSentPADODesc, stats.PPPoE.SentPADO)
	counter(pppoeRecvPADRDesc, stats.PPPoE.RecvPADR)
	counter(pppoeRecvPADRDupDesc, stats.PPPoE.RecvPADRDup)
	counter(pppoeSentPADSDesc, stats.PPPoE.SentPADS)
	counter(pppoeFilteredDesc, stats.PPPoE.Filtered)

	// RADIUS (per server). Absent servers are simply not emitted, so stale
	// series disappear automatically without any reset bookkeeping.
	for id, rs := range stats.RadiusServers {
		state := 0.0
		if rs.State == "active" {
			state = 1.0
		}
		rGauge := func(d *prometheus.Desc, v float64) {
			ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, id, rs.IP)
		}
		rCounter := func(d *prometheus.Desc, v float64) {
			ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v, id, rs.IP)
		}

		rGauge(radiusStateDesc, state)
		rCounter(radiusFailCountDesc, rs.FailCount)
		rGauge(radiusRequestCountDesc, rs.RequestCount)
		rGauge(radiusQueueLengthDesc, rs.QueueLength)
		rCounter(radiusAuthSentDesc, rs.AuthSent)
		rCounter(radiusAuthLostTotalDesc, rs.AuthLostTotal)
		rGauge(radiusAuthLost5mDesc, rs.AuthLost5m)
		rGauge(radiusAuthLost1mDesc, rs.AuthLost1m)
		rGauge(radiusAuthAvgTime5mDesc, rs.AuthAvgTime5m)
		rGauge(radiusAuthAvgTime1mDesc, rs.AuthAvgTime1m)
		rCounter(radiusAcctSentDesc, rs.AcctSent)
		rCounter(radiusAcctLostTotalDesc, rs.AcctLostTotal)
		rGauge(radiusAcctLost5mDesc, rs.AcctLost5m)
		rGauge(radiusAcctLost1mDesc, rs.AcctLost1m)
		rGauge(radiusAcctAvgTime5mDesc, rs.AcctAvgTime5m)
		rGauge(radiusAcctAvgTime1mDesc, rs.AcctAvgTime1m)
		rCounter(radiusInterimSentDesc, rs.InterimSent)
		rCounter(radiusInterimLostTotalDesc, rs.InterimLostTotal)
		rGauge(radiusInterimLost5mDesc, rs.InterimLost5m)
		rGauge(radiusInterimLost1mDesc, rs.InterimLost1m)
		rGauge(radiusInterimAvgTime5mDesc, rs.InterimAvgTime5m)
		rGauge(radiusInterimAvgTime1mDesc, rs.InterimAvgTime1m)
	}
}
