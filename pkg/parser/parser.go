// Package parser executes accel-cmd and parses its `show stat` output into
// typed statistics.
package parser

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"os/exec"
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
	RadiusServers map[string]RadiusStats
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
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, accelCmdPath, "show", "stat")
	// WaitDelay bounds how long Run blocks after the context is cancelled and the
	// process killed. Without it, a child that forks (e.g. a shell wrapper that
	// spawns a long-running grandchild) can inherit the stdout pipe and keep it
	// open, leaving Run stuck reading until that grandchild exits.
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return parseStats(out.String())
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
// destination untouched rather than recording partial data.
func fields(value string, want int) ([]float64, bool) {
	parts := strings.Split(value, "/")
	if len(parts) != want {
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
	// Example: "12345 / 67890 K"
	if v, ok := fields(strings.TrimSuffix(value, " K"), 2); ok {
		stats.MemRSS = v[0]
		stats.MemVirt = v[1]
	}
}

func parseCoreSection(core *CoreStats, key, value string) {
	switch key {
	case "mempool(allocated/available)":
		// Example: "1024 / 2048"
		if v, ok := fields(value, 2); ok {
			core.MempoolAllocated = v[0]
			core.MempoolAvailable = v[1]
		}
	case "threads(count/active)":
		if v, ok := fields(value, 2); ok {
			core.ThreadCount = v[0]
			core.ThreadActive = v[1]
		}
	case "context(count/sleep/pending)":
		if v, ok := fields(value, 3); ok {
			core.ContextCount = v[0]
			core.ContextSleeping = v[1]
			core.ContextPending = v[2]
		}
	case "md_handler(count/pending)":
		if v, ok := fields(value, 2); ok {
			core.MDHandlerCount = v[0]
			core.MDHandlerPending = v[1]
		}
	case "timer(count/pending)":
		if v, ok := fields(value, 2); ok {
			core.TimerCount = v[0]
			core.TimerPending = v[1]
		}
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

func parsePPPoESection(pppoe *PPPoEStats, key, value string) {
	f := atof(value)
	switch key {
	case "starting":
		pppoe.Starting = f
	case "active":
		pppoe.Active = f
	case "delayed PADO":
		pppoe.DelayedPADO = f
	case "recv PADI":
		pppoe.RecvPADI = f
	case "drop PADI":
		pppoe.DropPADI = f
	case "sent PADO":
		pppoe.SentPADO = f
	case "recv PADR":
		pppoe.RecvPADR = f
	case "recv PADR(dup)":
		pppoe.RecvPADRDup = f
	case "sent PADS":
		pppoe.SentPADS = f
	case "filtered":
		pppoe.Filtered = f
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
	case "auth avg time(5m/1m)":
		if v, ok := fields(value, 2); ok {
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
	case "acct avg time(5m/1m)":
		if v, ok := fields(value, 2); ok {
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
	case "interim avg time(5m/1m)":
		if v, ok := fields(value, 2); ok {
			radius.InterimAvgTime5m = v[0]
			radius.InterimAvgTime1m = v[1]
		}
	}
}
