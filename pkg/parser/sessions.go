package parser

import (
	"strconv"
	"strings"
	"time"
)

// sessionColumns are the "accel-cmd show sessions" columns the exporter reads.
// Deliberately no ifname, IP or delegated-prefix column: the ppp interface
// number is reused by unrelated sessions, and addresses change on every
// reconnect, so neither is a stable label for a subscriber.
const sessionColumns = "username,rate-limit,uptime-raw,rx-bytes-raw,tx-bytes-raw,rx-pkts,tx-pkts"

const sessionFieldCount = 7

// Session is one live accel-ppp session, as reported by "show sessions".
// The counters cover only the current session and restart at zero when the
// subscriber reconnects.
type Session struct {
	Username     string
	Uptime       float64 // seconds
	RxBytes      float64 // received from the subscriber
	TxBytes      float64 // sent to the subscriber
	RxPackets    float64
	TxPackets    float64
	RateDownKbit float64 // shaper limit towards the subscriber
	RateUpKbit   float64 // shaper limit from the subscriber
	HasRate      bool    // false when the session has no shaper limit
}

// RealmOf returns the realm of a username: the part after the first "#"
// ("user1#isp@vdsl" -> "isp@vdsl"), or "none" without one.
func RealmOf(username string) string {
	_, realm, found := strings.Cut(username, "#")
	if !found || realm == "" {
		return "none"
	}
	return realm
}

// DedupeSessions keeps one session per username (the newest, i.e. smallest
// uptime) and reports how many were dropped. A subscriber can briefly appear
// twice while reconnecting, and duplicate label sets are invalid for Prometheus.
func DedupeSessions(sessions []Session) ([]Session, int) {
	newest := make(map[string]int, len(sessions))
	out := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		i, seen := newest[s.Username]
		if !seen {
			newest[s.Username] = len(out)
			out = append(out, s)
			continue
		}
		if s.Uptime < out[i].Uptime {
			out[i] = s
		}
	}
	return out, len(sessions) - len(out)
}

// CollectSessions executes "accel-cmd show sessions" and parses its table
// output. It returns the sessions plus the number of lines that could not be
// parsed. The command is bounded by timeout like CollectStats.
func CollectSessions(accelCmdPath string, timeout time.Duration) ([]Session, int, error) {
	out, err := runAccelCmd(accelCmdPath, timeout, "show", "sessions", sessionColumns)
	if err != nil {
		return nil, 0, err
	}

	sessions, parseErrors := parseSessions(out)
	return sessions, parseErrors, nil
}

// parseSessions parses the table printed by "accel-cmd show sessions", skipping
// the header and separator lines. Unparseable lines are counted, not fatal.
func parseSessions(output string) ([]Session, int) {
	var sessions []Session
	parseErrors := 0
	for _, line := range strings.Split(output, "\n") {
		if isSessionsHeaderOrRule(line) {
			continue
		}
		s, ok := parseSessionRow(line)
		if !ok {
			parseErrors++
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, parseErrors
}

func isSessionsHeaderOrRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	if strings.Trim(trimmed, "-+ ") == "" {
		return true
	}
	first, _, _ := strings.Cut(trimmed, "|")
	return strings.TrimSpace(first) == "username"
}

func parseSessionRow(line string) (Session, bool) {
	fields := strings.Split(line, "|")
	if len(fields) != sessionFieldCount {
		return Session{}, false
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	if fields[0] == "" {
		return Session{}, false
	}

	s := Session{Username: fields[0]}
	s.RateDownKbit, s.RateUpKbit, s.HasRate = parseRateLimit(fields[1])

	nums := []*float64{&s.Uptime, &s.RxBytes, &s.TxBytes, &s.RxPackets, &s.TxPackets}
	for i, dst := range nums {
		v, err := strconv.ParseFloat(fields[2+i], 64)
		if err != nil {
			return Session{}, false
		}
		*dst = v
	}
	return s, true
}

// parseRateLimit reads accel-ppp's "down/up" shaper limit in Kbit/s. An empty
// value means the session is not shaped.
func parseRateLimit(value string) (down, up float64, ok bool) {
	d, u, found := strings.Cut(value, "/")
	if !found {
		return 0, 0, false
	}
	down, errDown := strconv.ParseFloat(strings.TrimSpace(d), 64)
	up, errUp := strconv.ParseFloat(strings.TrimSpace(u), 64)
	if errDown != nil || errUp != nil {
		return 0, 0, false
	}
	return down, up, true
}
