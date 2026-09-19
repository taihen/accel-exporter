// Package parser executes accel-cmd and parses its `show stat` output into
// typed statistics.
package parser

import (
	"bufio"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Stats represents all statistics gathered from accel-cmd
type Stats struct {
	Uptime        float64
	CPUPercent    float64
	MemRSS        float64
	MemVirt       float64
	Core          CoreStats
	Sessions      SessionStats
	PPPoE         PPPoEStats
	L2TP          L2TPStats
	RadiusServers map[string]RadiusStats
}

// L2TPStats contains the L2TP metrics from the "l2tp:" block of "show stat"
// (accel-pppd/ctrl/l2tp/l2tp.c, show_stat_exec).
type L2TPStats struct {
	Tunnels         TunnelSessionStats
	SessionsControl TunnelSessionStats
	SessionsData    TunnelSessionStats
}

// TunnelSessionStats is the starting/active/finishing triple reported for
// L2TP tunnels and for each session channel.
type TunnelSessionStats struct {
	Starting  float64
	Active    float64
	Finishing float64
}

// CoreStats contains core metrics
type CoreStats struct {
	MempoolAllocated float64
	MempoolAvailable float64
	ThreadCount      float64
	ThreadActive     float64
	ContextCount     float64
	ContextSleeping  float64
	ContextPending   float64
	MDHandlerCount   float64
	MDHandlerPending float64
	TimerCount       float64
	TimerPending     float64
}

// SessionStats contains session metrics
type SessionStats struct {
	Starting  float64
	Active    float64
	Finishing float64
}

// PPPoEStats contains PPPoE protocol metrics
type PPPoEStats struct {
	Starting    float64
	Active      float64
	DelayedPADO float64
	RecvPADI    float64
	DropPADI    float64
	SentPADO    float64
	RecvPADR    float64
	RecvPADRDup float64
	SentPADS    float64
	Filtered    float64
}

// RadiusStats contains RADIUS server metrics
type RadiusStats struct {
	ID               string
	IP               string
	State            string
	FailCount        float64
	RequestCount     float64
	QueueLength      float64
	AuthSent         float64
	AuthLostTotal    float64
	AuthLost5m       float64
	AuthLost1m       float64
	AuthAvgTime5m    float64
	AuthAvgTime1m    float64
	AcctSent         float64
	AcctLostTotal    float64
	AcctLost5m       float64
	AcctLost1m       float64
	AcctAvgTime5m    float64
	AcctAvgTime1m    float64
	InterimSent      float64
	InterimLostTotal float64
	InterimLost5m    float64
	InterimLost1m    float64
	InterimAvgTime5m float64
	InterimAvgTime1m float64
}

// CollectStats executes accel-cmd and parses its output. The command is bounded
// by timeout so a hung accel-cmd cannot wedge the scrape or leak processes; a
// non-positive timeout disables the deadline.
func CollectStats(accelCmdPath string, timeout time.Duration) (*Stats, error) {
	out, err := runAccelCmd(accelCmdPath, timeout, "show", "stat")
	if err != nil {
		return nil, err
	}

	return parseStats(out)
}

// parseStats parses the output of accel-cmd show stat
func parseStats(output string) (*Stats, error) {
	stats := &Stats{
		RadiusServers: make(map[string]RadiusStats),
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	var section string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch section {
		case "":
			parseMainSection(stats, key, value)
		case "core":
			parseCoreSection(&stats.Core, key, value)
		case "sessions":
			parseSessionsSection(&stats.Sessions, key, value)
		case "pppoe":
			parsePPPoESection(&stats.PPPoE, key, value)
		case "tunnels":
			parseTunnelSessionStats(&stats.L2TP.Tunnels, key, value)
		case "sessions (control channels)":
			parseTunnelSessionStats(&stats.L2TP.SessionsControl, key, value)
		case "sessions (data channels)":
			parseTunnelSessionStats(&stats.L2TP.SessionsData, key, value)
		default:
			if strings.HasPrefix(section, "radius") {
				radiusMatch := regexp.MustCompile(`radius\((\d+), ([\d\.]+)\)`).FindStringSubmatch(section)
				if len(radiusMatch) == 3 {
					radiusID := radiusMatch[1]
					radiusIP := radiusMatch[2]
					if _, exists := stats.RadiusServers[radiusID]; !exists {
						stats.RadiusServers[radiusID] = RadiusStats{
							ID: radiusID,
							IP: radiusIP,
						}
					}

					rs := stats.RadiusServers[radiusID]
					parseRadiusSection(&rs, key, value)
					stats.RadiusServers[radiusID] = rs
				}
			}
		}
	}

	return stats, scanner.Err()
}

// atof parses a numeric field. An empty value yields 0 silently (accel-cmd
// legitimately omits fields); a non-empty value that fails to parse yields 0
// but is logged, so malformed output is visible to operators instead of
// masquerading as a real zero.
func atof(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("parser: cannot parse %q as number: %v", value, err)
		return 0
	}
	return f
}

// fields splits a "/"-delimited value (e.g. "10 / 1 / 0") into want floats.
// It returns ok=false when the field count differs, so callers leave the
// destination untouched rather than recording partial data. An empty value is
// a silent miss (accel-cmd may omit a field); a non-empty value with the wrong
// count is logged so malformed output is visible to operators.
func fields(value string, want int) ([]float64, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, false
	}
	parts := strings.Split(value, "/")
	if len(parts) != want {
		log.Printf("parser: expected %d fields in %q, got %d", want, value, len(parts))
		return nil, false
	}
	out := make([]float64, want)
	for i, p := range parts {
		out[i] = atof(p)
	}
	return out, true
}

// Helper functions to parse each section...
func parseMainSection(stats *Stats, key, value string) {
	switch key {
	case "uptime":
		stats.Uptime = parseUptime(value)
	case "cpu":
		stats.CPUPercent = parsePercentage(value)
	case "mem(rss/virt)":
		parseMemory(stats, value)
	}
}

func parseUptime(value string) float64 {
	// Parse uptime in format "138.00:05:20" (days.hours:minutes:seconds)
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return 0
	}

	days, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}

	timeParts := strings.Split(parts[1], ":")
	if len(timeParts) != 3 {
		return 0
	}

	hours := atof(timeParts[0])
	minutes := atof(timeParts[1])
	seconds := atof(timeParts[2])

	return days*86400 + hours*3600 + minutes*60 + seconds
}

func parsePercentage(value string) float64 {
	// Example: "1.23%"
	return atof(strings.TrimSuffix(value, "%"))
}

func parseMemory(stats *Stats, value string) {
	// Real format (accel-pppd/cli/std_cmd.c: "mem(rss/virt): %lu/%lu kB"),
	// e.g. "10356/402548 kB" — confirmed against a live accel-cmd capture.
	// The trailing unit is "kB" (no leading space before "k"), not " K":
	// trimming the wrong suffix left it attached to the second field, which
	// then failed to parse as a number and silently zeroed MemVirt.
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "kB"))
	if v, ok := fields(value, 2); ok {
		stats.MemRSS = v[0]
		stats.MemVirt = v[1]
	}
}

func parseCoreSection(core *CoreStats, key, value string) {
	// Real format is flat single-value keys (accel-pppd/cli/std_cmd.c), e.g.
	// "mempool_allocated: 223472" — confirmed against a live accel-cmd
	// capture. The combined "mempool(allocated/available): 1024 / 2048"
	// shape below never matched any real accel-ppp output; every field in
	// this section was silently stuck at 0 as a result.
	f := atof(value)
	switch key {
	case "mempool_allocated":
		core.MempoolAllocated = f
	case "mempool_available":
		core.MempoolAvailable = f
	case "thread_count":
		core.ThreadCount = f
	case "thread_active":
		core.ThreadActive = f
	case "context_count":
		core.ContextCount = f
	case "context_sleeping":
		core.ContextSleeping = f
	case "context_pending":
		core.ContextPending = f
	case "md_handler_count":
		core.MDHandlerCount = f
	case "md_handler_pending":
		core.MDHandlerPending = f
	case "timer_count":
		core.TimerCount = f
	case "timer_pending":
		core.TimerPending = f
	}
}

func parseSessionsSection(sessions *SessionStats, key, value string) {
	f := atof(value)
	switch key {
	case "starting":
		sessions.Starting = f
	case "active":
		sessions.Active = f
	case "finishing":
		sessions.Finishing = f
	}
}

// padrDupPattern matches accel-ppp's combined "recv PADR(dup)" value, e.g.
// "5(2)" — see the case below.
var padrDupPattern = regexp.MustCompile(`^(\d+)\((\d+)\)$`)

func parsePPPoESection(pppoe *PPPoEStats, key, value string) {
	switch key {
	case "starting":
		pppoe.Starting = atof(value)
	case "active":
		pppoe.Active = atof(value)
	case "delayed PADO":
		pppoe.DelayedPADO = atof(value)
	case "recv PADI":
		pppoe.RecvPADI = atof(value)
	case "drop PADI":
		pppoe.DropPADI = atof(value)
	case "sent PADO":
		pppoe.SentPADO = atof(value)
	case "recv PADR(dup)":
		// Real accel-ppp emits this as one combined line (accel-pppd/ctrl/
		// pppoe/cli.c: "recv PADR(dup): %lu(%lu)", e.g. "5(2)"), not two
		// separate "recv PADR"/"recv PADR(dup)" lines — "recv PADR" alone
		// never appears in real output, and the old plain-atof parse of
		// this combined value always failed and logged an error.
		if m := padrDupPattern.FindStringSubmatch(value); m != nil {
			pppoe.RecvPADR = atof(m[1])
			pppoe.RecvPADRDup = atof(m[2])
		} else {
			log.Printf("parser: cannot parse %q as \"n(dup)\"", value)
		}
	case "sent PADS":
		pppoe.SentPADS = atof(value)
	case "filtered":
		pppoe.Filtered = atof(value)
	}
}

func parseRadiusSection(radius *RadiusStats, key, value string) {
	switch key {
	case "state":
		radius.State = value // State is a string
	case "fail count":
		radius.FailCount = atof(value)
	case "request count":
		radius.RequestCount = atof(value)
	case "queue length":
		radius.QueueLength = atof(value)
	case "auth sent":
		radius.AuthSent = atof(value)
	case "auth lost(total/5m/1m)":
		if v, ok := fields(value, 3); ok {
			radius.AuthLostTotal = v[0]
			radius.AuthLost5m = v[1]
			radius.AuthLost1m = v[2]
		}
	case "auth avg query time(5m/1m)":
		// Real key includes "query" (accel-pppd/radius/serv.c: "auth avg
		// query time(5m/1m): %lu/%lu ms") and the value carries a trailing
		// " ms" unit the old code never stripped — both the wrong key and
		// the unstripped unit meant this never matched real output.
		// Values are milliseconds; AuthAvgTime5m/1m stay milliseconds here,
		// the collector converts to seconds to match the metric name.
		if v, ok := fields(strings.TrimSuffix(value, " ms"), 2); ok {
			radius.AuthAvgTime5m = v[0]
			radius.AuthAvgTime1m = v[1]
		}
	case "acct sent":
		radius.AcctSent = atof(value)
	case "acct lost(total/5m/1m)":
		if v, ok := fields(value, 3); ok {
			radius.AcctLostTotal = v[0]
			radius.AcctLost5m = v[1]
			radius.AcctLost1m = v[2]
		}
	case "acct avg query time(5m/1m)":
		// Same "query" + trailing " ms" fix as auth above.
		if v, ok := fields(strings.TrimSuffix(value, " ms"), 2); ok {
			radius.AcctAvgTime5m = v[0]
			radius.AcctAvgTime1m = v[1]
		}
	case "interim sent":
		radius.InterimSent = atof(value)
	case "interim lost(total/5m/1m)":
		if v, ok := fields(value, 3); ok {
			radius.InterimLostTotal = v[0]
			radius.InterimLost5m = v[1]
			radius.InterimLost1m = v[2]
		}
	case "interim avg query time(5m/1m)":
		// Same "query" + trailing " ms" fix as auth above.
		if v, ok := fields(strings.TrimSuffix(value, " ms"), 2); ok {
			radius.InterimAvgTime5m = v[0]
			radius.InterimAvgTime1m = v[1]
		}
	}
}

func parseTunnelSessionStats(s *TunnelSessionStats, key, value string) {
	f := atof(value)
	switch key {
	case "starting":
		s.Starting = f
	case "active":
		s.Active = f
	case "finishing":
		s.Finishing = f
	}
}
