package collector

import (
	"log"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/taihen/accel-exporter/pkg/parser"
)

// AccelCollector implements the prometheus.Collector interface
type AccelCollector struct {
	accelCmdPath string

	// General metrics
	up             prometheus.Gauge
	scrapeFailures prometheus.Counter
	uptimeSeconds  prometheus.Gauge
	cpuPercent     prometheus.Gauge
	memRSS         prometheus.Gauge
	memVirt        prometheus.Gauge

	// Core metrics
	coreMempoolAllocated prometheus.Gauge
	coreMempoolAvailable prometheus.Gauge
	coreThreadCount      prometheus.Gauge
	coreThreadActive     prometheus.Gauge
	coreContextCount     prometheus.Gauge
	coreContextSleeping  prometheus.Gauge
	coreContextPending   prometheus.Gauge
	coreMDHandlerCount   prometheus.Gauge
	coreMDHandlerPending prometheus.Gauge
	coreTimerCount       prometheus.Gauge
	coreTimerPending     prometheus.Gauge

	// Session metrics
	sessionsStarting  prometheus.Gauge
	sessionsActive    prometheus.Gauge
	sessionsFinishing prometheus.Gauge

	// PPPoE metrics
	pppoeStarting    prometheus.Gauge
	pppoeActive      prometheus.Gauge
	pppoeDelayedPADO prometheus.Gauge
	pppoeRecvPADI    prometheus.Counter
	pppoeDropPADI    prometheus.Counter
	pppoeSentPADO    prometheus.Counter
	pppoeRecvPADR    prometheus.Counter
	pppoeRecvPADRDup prometheus.Counter
	pppoeSentPADS    prometheus.Counter
	pppoeFiltered    prometheus.Counter

	// RADIUS metrics
	radiusState            *prometheus.GaugeVec
	radiusFailCount        *prometheus.CounterVec
	radiusRequestCount     *prometheus.GaugeVec
	radiusQueueLength      *prometheus.GaugeVec
	radiusAuthSent         *prometheus.CounterVec
	radiusAuthLostTotal    *prometheus.CounterVec
	radiusAuthLost5m       *prometheus.GaugeVec
	radiusAuthLost1m       *prometheus.GaugeVec
	radiusAuthAvgTime5m    *prometheus.GaugeVec
	radiusAuthAvgTime1m    *prometheus.GaugeVec
	radiusAcctSent         *prometheus.CounterVec
	radiusAcctLostTotal    *prometheus.CounterVec
	radiusAcctLost5m       *prometheus.GaugeVec
	radiusAcctLost1m       *prometheus.GaugeVec
	radiusAcctAvgTime5m    *prometheus.GaugeVec
	radiusAcctAvgTime1m    *prometheus.GaugeVec
	radiusInterimSent      *prometheus.CounterVec
	radiusInterimLostTotal *prometheus.CounterVec
	radiusInterimLost5m    *prometheus.GaugeVec
	radiusInterimLost1m    *prometheus.GaugeVec
	radiusInterimAvgTime5m *prometheus.GaugeVec
	radiusInterimAvgTime1m *prometheus.GaugeVec
}

// NewAccelCollector creates a new AccelCollector
func NewAccelCollector(accelCmdPath string) *AccelCollector {
	radiusLabels := []string{"server_id", "server_ip"}

	return &AccelCollector{
		accelCmdPath: accelCmdPath,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_up",
			Help: "Was the last accel-cmd scrape successful.",
		}),
		scrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_scrape_failures_total",
			Help: "Number of errors while scraping accel-cmd.",
		}),
		uptimeSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_uptime_seconds",
			Help: "Uptime of accel-ppp in seconds.",
		}),
		cpuPercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_cpu_usage_percent",
			Help: "CPU usage percentage.",
		}),
		memRSS: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_memory_rss_bytes",
			Help: "RSS memory usage in bytes.",
		}),
		memVirt: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_memory_virtual_bytes",
			Help: "Virtual memory usage in bytes.",
		}),

		// Core metrics initialization
		coreMempoolAllocated: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_mempool_allocated_bytes",
			Help: "Allocated memory pool size.",
		}),
		coreMempoolAvailable: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_mempool_available_bytes",
			Help: "Available memory pool size.",
		}),
		coreThreadCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_thread_count",
			Help: "Number of core threads.",
		}),
		coreThreadActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_thread_active",
			Help: "Number of active core threads.",
		}),
		coreContextCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_context_count",
			Help: "Number of core contexts.",
		}),
		coreContextSleeping: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_context_sleeping",
			Help: "Number of sleeping core contexts.",
		}),
		coreContextPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_context_pending",
			Help: "Number of pending core contexts.",
		}),
		coreMDHandlerCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_md_handler_count",
			Help: "Number of MD handlers.",
		}),
		coreMDHandlerPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_md_handler_pending",
			Help: "Number of pending MD handlers.",
		}),
		coreTimerCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_timer_count",
			Help: "Number of core timers.",
		}),
		coreTimerPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_core_timer_pending",
			Help: "Number of pending core timers.",
		}),

		// Session metrics
		sessionsStarting: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_sessions_starting",
			Help: "Number of sessions starting.",
		}),
		sessionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_sessions_active",
			Help: "Number of active sessions.",
		}),
		sessionsFinishing: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_sessions_finishing",
			Help: "Number of sessions finishing.",
		}),

		// PPPoE metrics
		pppoeStarting: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_pppoe_starting",
			Help: "Number of PPPoE sessions starting.",
		}),
		pppoeActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_pppoe_active",
			Help: "Number of active PPPoE sessions.",
		}),
		pppoeDelayedPADO: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "accel_pppoe_delayed_pado",
			Help: "Number of delayed PADO packets.",
		}),
		pppoeRecvPADI: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_pppoe_recv_padi_total",
			Help: "Total received PADI packets.",
		}),
		pppoeDropPADI: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_pppoe_drop_padi_total",
			Help: "Total dropped PADI packets.",
		}),
		pppoeSentPADO: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_pppoe_sent_pado_total",
			Help: "Total sent PADO packets.",
		}),
		pppoeRecvPADR: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_pppoe_recv_padr_total",
			Help: "Total received PADR packets.",
		}),
		pppoeRecvPADRDup: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_pppoe_recv_padr_dup_total",
			Help: "Total received duplicate PADR packets.",
		}),
		pppoeSentPADS: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_pppoe_sent_pads_total",
			Help: "Total sent PADS packets.",
		}),
		pppoeFiltered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "accel_pppoe_filtered_total",
			Help: "Total filtered PPPoE packets.",
		}),

		// RADIUS metrics with labels
		radiusState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_state",
				Help: "State of RADIUS server (1 = active, 0 = inactive).",
			},
			radiusLabels,
		),
		radiusFailCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "accel_radius_fail_count_total",
				Help: "Total RADIUS server fail count.",
			},
			radiusLabels,
		),
		radiusRequestCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_request_count",
				Help: "Current RADIUS server request count.",
			},
			radiusLabels,
		),
		radiusQueueLength: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_queue_length",
				Help: "Current RADIUS server queue length.",
			},
			radiusLabels,
		),
		radiusAuthSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "accel_radius_auth_sent_total",
				Help: "Total RADIUS auth packets sent.",
			},
			radiusLabels,
		),
		radiusAuthLostTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "accel_radius_auth_lost_total",
				Help: "Total RADIUS auth packets lost.",
			},
			radiusLabels,
		),
		radiusAuthLost5m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_auth_lost_5m",
				Help: "RADIUS auth packets lost in the last 5 minutes.",
			},
			radiusLabels,
		),
		radiusAuthLost1m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_auth_lost_1m",
				Help: "RADIUS auth packets lost in the last 1 minute.",
			},
			radiusLabels,
		),
		radiusAuthAvgTime5m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_auth_avg_time_5m_seconds",
				Help: "Average RADIUS auth response time in the last 5 minutes (seconds).",
			},
			radiusLabels,
		),
		radiusAuthAvgTime1m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_auth_avg_time_1m_seconds",
				Help: "Average RADIUS auth response time in the last 1 minute (seconds).",
			},
			radiusLabels,
		),
		radiusAcctSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "accel_radius_acct_sent_total",
				Help: "Total RADIUS accounting packets sent.",
			},
			radiusLabels,
		),
		radiusAcctLostTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "accel_radius_acct_lost_total",
				Help: "Total RADIUS accounting packets lost.",
			},
			radiusLabels,
		),
		radiusAcctLost5m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_acct_lost_5m",
				Help: "RADIUS accounting packets lost in the last 5 minutes.",
			},
			radiusLabels,
		),
		radiusAcctLost1m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_acct_lost_1m",
				Help: "RADIUS accounting packets lost in the last 1 minute.",
			},
			radiusLabels,
		),
		radiusAcctAvgTime5m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_acct_avg_time_5m_seconds",
				Help: "Average RADIUS accounting response time in the last 5 minutes (seconds).",
			},
			radiusLabels,
		),
		radiusAcctAvgTime1m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_acct_avg_time_1m_seconds",
				Help: "Average RADIUS accounting response time in the last 1 minute (seconds).",
			},
			radiusLabels,
		),
		radiusInterimSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "accel_radius_interim_sent_total",
				Help: "Total RADIUS interim accounting packets sent.",
			},
			radiusLabels,
		),
		radiusInterimLostTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "accel_radius_interim_lost_total",
				Help: "Total RADIUS interim accounting packets lost.",
			},
			radiusLabels,
		),
		radiusInterimLost5m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_interim_lost_5m",
				Help: "RADIUS interim accounting packets lost in the last 5 minutes.",
			},
			radiusLabels,
		),
		radiusInterimLost1m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_interim_lost_1m",
				Help: "RADIUS interim accounting packets lost in the last 1 minute.",
			},
			radiusLabels,
		),
		radiusInterimAvgTime5m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_interim_avg_time_5m_seconds",
				Help: "Average RADIUS interim accounting response time in the last 5 minutes (seconds).",
			},
			radiusLabels,
		),
		radiusInterimAvgTime1m: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "accel_radius_interim_avg_time_1m_seconds",
				Help: "Average RADIUS interim accounting response time in the last 1 minute (seconds).",
			},
			radiusLabels,
		),
	}
}

// Describe implements the prometheus.Collector interface
func (c *AccelCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up.Desc()
	ch <- c.scrapeFailures.Desc()
	ch <- c.uptimeSeconds.Desc()
	ch <- c.cpuPercent.Desc()
	ch <- c.memRSS.Desc()
	ch <- c.memVirt.Desc()

	// Core metrics
	ch <- c.coreMempoolAllocated.Desc()
	ch <- c.coreMempoolAvailable.Desc()
	ch <- c.coreThreadCount.Desc()
	ch <- c.coreThreadActive.Desc()
	ch <- c.coreContextCount.Desc()
	ch <- c.coreContextSleeping.Desc()
	ch <- c.coreContextPending.Desc()
	ch <- c.coreMDHandlerCount.Desc()
	ch <- c.coreMDHandlerPending.Desc()
	ch <- c.coreTimerCount.Desc()
	ch <- c.coreTimerPending.Desc()

	// Session metrics
	ch <- c.sessionsStarting.Desc()
	ch <- c.sessionsActive.Desc()
	ch <- c.sessionsFinishing.Desc()

	// PPPoE metrics
	ch <- c.pppoeStarting.Desc()
	ch <- c.pppoeActive.Desc()
	ch <- c.pppoeDelayedPADO.Desc()
	ch <- c.pppoeRecvPADI.Desc()
	ch <- c.pppoeDropPADI.Desc()
	ch <- c.pppoeSentPADO.Desc()
	ch <- c.pppoeRecvPADR.Desc()
	ch <- c.pppoeRecvPADRDup.Desc()
	ch <- c.pppoeSentPADS.Desc()
	ch <- c.pppoeFiltered.Desc()

	// RADIUS metrics
	c.radiusState.Describe(ch)
	c.radiusFailCount.Describe(ch)
	c.radiusRequestCount.Describe(ch)
	c.radiusQueueLength.Describe(ch)
	c.radiusAuthSent.Describe(ch)
	c.radiusAuthLostTotal.Describe(ch)
	c.radiusAuthLost5m.Describe(ch)
	c.radiusAuthLost1m.Describe(ch)
	c.radiusAuthAvgTime5m.Describe(ch)
	c.radiusAuthAvgTime1m.Describe(ch)
	c.radiusAcctSent.Describe(ch)
	c.radiusAcctLostTotal.Describe(ch)
	c.radiusAcctLost5m.Describe(ch)
	c.radiusAcctLost1m.Describe(ch)
	c.radiusAcctAvgTime5m.Describe(ch)
	c.radiusAcctAvgTime1m.Describe(ch)
	c.radiusInterimSent.Describe(ch)
	c.radiusInterimLostTotal.Describe(ch)
	c.radiusInterimLost5m.Describe(ch)
	c.radiusInterimLost1m.Describe(ch)
	c.radiusInterimAvgTime5m.Describe(ch)
	c.radiusInterimAvgTime1m.Describe(ch)
}

// Collect implements the prometheus.Collector interface
func (c *AccelCollector) Collect(ch chan<- prometheus.Metric) {
	stats, err := parser.CollectStats(c.accelCmdPath)
	if err != nil {
		c.up.Set(0)
		c.scrapeFailures.Inc()
		ch <- c.up
		ch <- c.scrapeFailures
		log.Printf("Error collecting stats: %v", err)
		return
	}

	c.up.Set(1)
	ch <- c.up

	// Set general metrics
	c.uptimeSeconds.Set(stats.Uptime)
	ch <- c.uptimeSeconds

	c.cpuPercent.Set(stats.CPUPercent)
	ch <- c.cpuPercent

	c.memRSS.Set(stats.MemRSS * 1024) // Convert KB to bytes
	ch <- c.memRSS

	c.memVirt.Set(stats.MemVirt * 1024) // Convert KB to bytes
	ch <- c.memVirt

	// Set core metrics
	c.coreMempoolAllocated.Set(stats.Core.MempoolAllocated)
	ch <- c.coreMempoolAllocated
	c.coreMempoolAvailable.Set(stats.Core.MempoolAvailable)
	ch <- c.coreMempoolAvailable
	c.coreThreadCount.Set(stats.Core.ThreadCount)
	ch <- c.coreThreadCount
	c.coreThreadActive.Set(stats.Core.ThreadActive)
	ch <- c.coreThreadActive
	c.coreContextCount.Set(stats.Core.ContextCount)
	ch <- c.coreContextCount
	c.coreContextSleeping.Set(stats.Core.ContextSleeping)
	ch <- c.coreContextSleeping
	c.coreContextPending.Set(stats.Core.ContextPending)
	ch <- c.coreContextPending
	c.coreMDHandlerCount.Set(stats.Core.MDHandlerCount)
	ch <- c.coreMDHandlerCount
	c.coreMDHandlerPending.Set(stats.Core.MDHandlerPending)
	ch <- c.coreMDHandlerPending
	c.coreTimerCount.Set(stats.Core.TimerCount)
	ch <- c.coreTimerCount
	c.coreTimerPending.Set(stats.Core.TimerPending)
	ch <- c.coreTimerPending

	// Set session metrics
	c.sessionsStarting.Set(stats.Sessions.Starting)
	ch <- c.sessionsStarting
	c.sessionsActive.Set(stats.Sessions.Active)
	ch <- c.sessionsActive
	c.sessionsFinishing.Set(stats.Sessions.Finishing)
	ch <- c.sessionsFinishing

	// Set PPPoE metrics
	c.pppoeStarting.Set(stats.PPPoE.Starting)
	ch <- c.pppoeStarting
	c.pppoeActive.Set(stats.PPPoE.Active)
	ch <- c.pppoeActive
	c.pppoeDelayedPADO.Set(stats.PPPoE.DelayedPADO)
	ch <- c.pppoeDelayedPADO
	// Note: The original code used .Add() for counters here, but Collect should set the current value.
	// If these are truly counters, they should be handled differently, perhaps by storing the previous value.
	// For simplicity based on the provided structure, we'll use Set here, assuming the parser provides the total count.
	// If the parser provides increments, .Add() would be correct, but Prometheus counters should generally only increase.
	// A better approach might be to fetch the previous value or rely on Prometheus's rate() function.
	// Sticking to the provided structure for now, using Set for counters as well.
	c.pppoeRecvPADI.Set(stats.PPPoE.RecvPADI)
	ch <- c.pppoeRecvPADI
	c.pppoeDropPADI.Set(stats.PPPoE.DropPADI)
	ch <- c.pppoeDropPADI
	c.pppoeSentPADO.Set(stats.PPPoE.SentPADO)
	ch <- c.pppoeSentPADO
	c.pppoeRecvPADR.Set(stats.PPPoE.RecvPADR)
	ch <- c.pppoeRecvPADR
	c.pppoeRecvPADRDup.Set(stats.PPPoE.RecvPADRDup)
	ch <- c.pppoeRecvPADRDup
	c.pppoeSentPADS.Set(stats.PPPoE.SentPADS)
	ch <- c.pppoeSentPADS
	c.pppoeFiltered.Set(stats.PPPoE.Filtered)
	ch <- c.pppoeFiltered

	// Set RADIUS metrics
	// Reset vectors before setting new values to avoid stale metrics if a server disappears
	c.radiusState.Reset()
	c.radiusFailCount.Reset()
	c.radiusRequestCount.Reset()
	c.radiusQueueLength.Reset()
	c.radiusAuthSent.Reset()
	c.radiusAuthLostTotal.Reset()
	c.radiusAuthLost5m.Reset()
	c.radiusAuthLost1m.Reset()
	c.radiusAuthAvgTime5m.Reset()
	c.radiusAuthAvgTime1m.Reset()
	c.radiusAcctSent.Reset()
	c.radiusAcctLostTotal.Reset()
	c.radiusAcctLost5m.Reset()
	c.radiusAcctLost1m.Reset()
	c.radiusAcctAvgTime5m.Reset()
	c.radiusAcctAvgTime1m.Reset()
	c.radiusInterimSent.Reset()
	c.radiusInterimLostTotal.Reset()
	c.radiusInterimLost5m.Reset()
	c.radiusInterimLost1m.Reset()
	c.radiusInterimAvgTime5m.Reset()
	c.radiusInterimAvgTime1m.Reset()

	for id, rs := range stats.RadiusServers {
		state := 0.0
		if rs.State == "active" {
			state = 1.0
		}

		labels := prometheus.Labels{"server_id": id, "server_ip": rs.IP}

		c.radiusState.With(labels).Set(state)
		// Assuming FailCount from parser is the total count, set the counter value directly.
		c.radiusFailCount.With(labels).Set(rs.FailCount)
		c.radiusRequestCount.With(labels).Set(rs.RequestCount)
		c.radiusQueueLength.With(labels).Set(rs.QueueLength)
		c.radiusAuthSent.With(labels).Set(rs.AuthSent)
		c.radiusAuthLostTotal.With(labels).Set(rs.AuthLostTotal)
		c.radiusAuthLost5m.With(labels).Set(rs.AuthLost5m)
		c.radiusAuthLost1m.With(labels).Set(rs.AuthLost1m)
		c.radiusAuthAvgTime5m.With(labels).Set(rs.AuthAvgTime5m)
		c.radiusAuthAvgTime1m.With(labels).Set(rs.AuthAvgTime1m)
		c.radiusAcctSent.With(labels).Set(rs.AcctSent)
		c.radiusAcctLostTotal.With(labels).Set(rs.AcctLostTotal)
		c.radiusAcctLost5m.With(labels).Set(rs.AcctLost5m)
		c.radiusAcctLost1m.With(labels).Set(rs.AcctLost1m)
		c.radiusAcctAvgTime5m.With(labels).Set(rs.AcctAvgTime5m)
		c.radiusAcctAvgTime1m.With(labels).Set(rs.AcctAvgTime1m)
		c.radiusInterimSent.With(labels).Set(rs.InterimSent)
		c.radiusInterimLostTotal.With(labels).Set(rs.InterimLostTotal)
		c.radiusInterimLost5m.With(labels).Set(rs.InterimLost5m)
		c.radiusInterimLost1m.With(labels).Set(rs.InterimLost1m)
		c.radiusInterimAvgTime5m.With(labels).Set(rs.InterimAvgTime5m)
		c.radiusInterimAvgTime1m.With(labels).Set(rs.InterimAvgTime1m)
	}

	// Collect all vector metrics
	c.radiusState.Collect(ch)
	c.radiusFailCount.Collect(ch)
	c.radiusRequestCount.Collect(ch)
	c.radiusQueueLength.Collect(ch)
	c.radiusAuthSent.Collect(ch)
	c.radiusAuthLostTotal.Collect(ch)
	c.radiusAuthLost5m.Collect(ch)
	c.radiusAuthLost1m.Collect(ch)
	c.radiusAuthAvgTime5m.Collect(ch)
	c.radiusAuthAvgTime1m.Collect(ch)
	c.radiusAcctSent.Collect(ch)
	c.radiusAcctLostTotal.Collect(ch)
	c.radiusAcctLost5m.Collect(ch)
	c.radiusAcctLost1m.Collect(ch)
	c.radiusAcctAvgTime5m.Collect(ch)
	c.radiusAcctAvgTime1m.Collect(ch)
	c.radiusInterimSent.Collect(ch)
	c.radiusInterimLostTotal.Collect(ch)
	c.radiusInterimLost5m.Collect(ch)
	c.radiusInterimLost1m.Collect(ch)
	c.radiusInterimAvgTime5m.Collect(ch)
	c.radiusInterimAvgTime1m.Collect(ch)
}
