package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
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

// CollectStats executes accel-cmd or uses telnet and parses its output
func CollectStats(accelCmdPath, accelCmdPwd, accelHost string, accelPort int) (*Stats, error) {
	if accelHost != "" && accelPort > 0 {
		return collectViaTelnet(accelHost, accelPort, accelCmdPwd)
	}
	return collectViaCmd(accelCmdPath, accelCmdPwd)
}

func collectViaCmd(accelCmdPath, accelCmdPwd string) (*Stats, error) {
	var args []string
	if accelCmdPwd != "" {
		args = []string{"--password", accelCmdPwd, "show", "stat"}
	} else {
		args = []string{"show", "stat"}
	}

	cmd := exec.Command(accelCmdPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return parseStats(out.String())
}

func collectViaTelnet(host string, port int, password string) (*Stats, error) {
	// Connect to the remote host using TCP with a 5-second timeout
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close() // Ensure connection is closed when function exits

	var out bytes.Buffer
	buf := make([]byte, 4096)

	passwordSent := false
	commandSent := false

	// Set an initial read deadline to avoid blocking indefinitely
	conn.SetDeadline(time.Now().Add(8 * time.Second))

	for {
		n, err := conn.Read(buf) // Read data from the TCP connection
		if n > 0 {
			chunk := string(buf[:n])
			out.WriteString(chunk)

			// 🔍 Temporary debug to print received data
			//fmt.Println("RECV:", chunk)

			// If a Password prompt is detected and we haven't sent the password yet
			if !passwordSent && strings.Contains(chunk, "Password:") {
				if password != "" {
					// Send the password followed by CRLF (\r\n), which telnet expects
					_, _ = conn.Write([]byte(password + "\r\n"))
				}
				passwordSent = true
				// Extend the read deadline after sending the password
				conn.SetDeadline(time.Now().Add(8 * time.Second))
			}

			// If the command prompt is detected and we haven't sent the command yet
			if !commandSent && strings.Contains(chunk, "accel-ppp#") {
				// Send the "show stat" command followed by CRLF
				_, _ = conn.Write([]byte("show stat\r\n"))
				commandSent = true
				// Extend the read deadline for waiting the command output
				conn.SetDeadline(time.Now().Add(8 * time.Second))
				continue
			}

			// If we already sent the command and detect the prompt again, we're done
			if commandSent && strings.Contains(chunk, "accel-ppp#") {
				break
			}
		}

		if err != nil {
			if err == io.EOF {
				break // Connection closed by remote host
			}
			return nil, err // Return other errors
		}
	}

	// Parse the accumulated output and return the stats
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

	hours, _ := strconv.ParseFloat(timeParts[0], 64)
	minutes, _ := strconv.ParseFloat(timeParts[1], 64)
	seconds, _ := strconv.ParseFloat(timeParts[2], 64)

	return days*86400 + hours*3600 + minutes*60 + seconds
}

func parsePercentage(value string) float64 {
	// Example: "1.23%"
	trimmed := strings.TrimSuffix(value, "%")
	f, _ := strconv.ParseFloat(trimmed, 64)
	return f
}

func parseMemory(stats *Stats, value string) {
	// Example: "12345 / 67890 K"
	parts := strings.Split(strings.TrimSuffix(value, " K"), "/")
	if len(parts) == 2 {
		rss, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		virt, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		stats.MemRSS = rss
		stats.MemVirt = virt
	}
}

func parseCoreSection(core *CoreStats, key, value string) {
	// Implement parsing logic based on key
	switch key {
	case "mempool(allocated/available)":
		// Example: "1024 / 2048"
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			alloc, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			avail, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			core.MempoolAllocated = alloc
			core.MempoolAvailable = avail
		}
	case "threads(count/active)":
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			count, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			active, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			core.ThreadCount = count
			core.ThreadActive = active
		}
	case "context(count/sleep/pending)":
		parts := strings.Split(value, "/")
		if len(parts) == 3 {
			count, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			sleep, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			pending, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
			core.ContextCount = count
			core.ContextSleeping = sleep
			core.ContextPending = pending
		}
	case "md_handler(count/pending)":
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			count, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			pending, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			core.MDHandlerCount = count
			core.MDHandlerPending = pending
		}
	case "timer(count/pending)":
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			count, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			pending, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			core.TimerCount = count
			core.TimerPending = pending
		}
	}
}

func parseSessionsSection(sessions *SessionStats, key, value string) {
	f, _ := strconv.ParseFloat(value, 64)
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
	f, _ := strconv.ParseFloat(value, 64)
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
	f, _ := strconv.ParseFloat(value, 64)
	switch key {
	case "state":
		radius.State = value // State is a string
	case "fail count":
		radius.FailCount = f
	case "request count":
		radius.RequestCount = f
	case "queue length":
		radius.QueueLength = f
	case "auth sent":
		radius.AuthSent = f
	case "auth lost(total/5m/1m)":
		parts := strings.Split(value, "/")
		if len(parts) == 3 {
			total, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			m5, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			m1, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
			radius.AuthLostTotal = total
			radius.AuthLost5m = m5
			radius.AuthLost1m = m1
		}
	case "auth avg time(5m/1m)":
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			m5, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			m1, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			radius.AuthAvgTime5m = m5
			radius.AuthAvgTime1m = m1
		}
	case "acct sent":
		radius.AcctSent = f
	case "acct lost(total/5m/1m)":
		parts := strings.Split(value, "/")
		if len(parts) == 3 {
			total, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			m5, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			m1, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
			radius.AcctLostTotal = total
			radius.AcctLost5m = m5
			radius.AcctLost1m = m1
		}
	case "acct avg time(5m/1m)":
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			m5, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			m1, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			radius.AcctAvgTime5m = m5
			radius.AcctAvgTime1m = m1
		}
	case "interim sent":
		radius.InterimSent = f
	case "interim lost(total/5m/1m)":
		parts := strings.Split(value, "/")
		if len(parts) == 3 {
			total, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			m5, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			m1, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
			radius.InterimLostTotal = total
			radius.InterimLost5m = m5
			radius.InterimLost1m = m1
		}
	case "interim avg time(5m/1m)":
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			m5, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			m1, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			radius.InterimAvgTime5m = m5
			radius.InterimAvgTime1m = m1
		}
	}
}
