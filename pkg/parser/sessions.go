package parser

import (
	"context"
	"strconv"
	"strings"
)

// sessionColumns are the "accel-cmd show sessions" columns the exporter reads.
// sid identifies a session: the same username can have several live sessions
// (accel-ppp allows that unless single-session is set). Deliberately no
// ifname, IP or delegated-prefix column: the ppp interface number is reused by
// unrelated sessions and addresses change on every reconnect.
//
// rate-limit is registered by the shaper module. Without it accel-ppp rejects
// the whole command ("unknown column"), so no session metrics are exported.
const sessionColumns = "sid,username,rate-limit,uptime-raw,rx-bytes-raw,tx-bytes-raw,rx-pkts,tx-pkts"

const sessionFieldCount = 8

// Session is one live accel-ppp session, as reported by "show sessions".
// The counters cover only this session and start at zero when it starts.
type Session struct {
	SID          string
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

// CollectSessions executes "accel-cmd show sessions" and parses its table
// output. It returns the sessions plus the number of lines that could not be
// parsed. The command is bounded by ctx like CollectStats.
func CollectSessions(ctx context.Context, accelCmdPath string) ([]Session, int, error) {
	out, err := runAccelCmd(ctx, accelCmdPath, "show", "sessions", sessionColumns)
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
	return strings.TrimSpace(first) == "sid"
}

func parseSessionRow(line string) (Session, bool) {
	fields := strings.Split(line, "|")
	if len(fields) != sessionFieldCount {
		return Session{}, false
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	if fields[0] == "" || fields[1] == "" {
		return Session{}, false
	}

	s := Session{SID: fields[0], Username: fields[1]}
	s.RateDownKbit, s.RateUpKbit, s.HasRate = parseRateLimit(fields[2])

	nums := []*float64{&s.Uptime, &s.RxBytes, &s.TxBytes, &s.RxPackets, &s.TxPackets}
	for i, dst := range nums {
		v, err := strconv.ParseFloat(fields[3+i], 64)
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
