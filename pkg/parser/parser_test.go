package parser

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeAccelCmd writes an executable shell script to a temp dir that runs body,
// and returns its path. Skips on non-POSIX platforms.
func fakeAccelCmd(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake not supported on windows")
	}
	path := filepath.Join(t.TempDir(), "accel-cmd")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	return path
}

func TestCollectStatsSuccess(t *testing.T) {
	path := fakeAccelCmd(t, "cat <<'EOF'\n"+sampleStat+"EOF")
	st, err := CollectStats(path, time.Second)
	if err != nil {
		t.Fatalf("CollectStats: %v", err)
	}
	wantEq(t, "CPUPercent", st.CPUPercent, 1.50)
	wantEq(t, "Sessions.Active", st.Sessions.Active, 100)
	if len(st.RadiusServers) != 1 {
		t.Fatalf("RadiusServers = %d, want 1", len(st.RadiusServers))
	}
}

// TestCollectStatsTimeout proves a hung accel-cmd is killed at the deadline
// instead of wedging the scrape forever.
func TestCollectStatsTimeout(t *testing.T) {
	path := fakeAccelCmd(t, "sleep 5")
	start := time.Now()
	if _, err := CollectStats(path, 50*time.Millisecond); err == nil {
		t.Fatal("CollectStats: want timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("CollectStats blocked %v, timeout not enforced", elapsed)
	}
}

func TestCollectStatsExecError(t *testing.T) {
	if _, err := CollectStats("/nonexistent/accel-cmd-xyz", time.Second); err == nil {
		t.Fatal("CollectStats: want exec error, got nil")
	}
}

// sampleStat is a representative `accel-cmd show stat` capture exercising every
// section the parser understands: the unlabelled main block, core, sessions,
// pppoe, and a single radius server. Indentation is intentional — the parser
// trims each line, so leading whitespace must not change the result.
const sampleStat = `uptime: 138.00:05:20
cpu: 1.50%
mem(rss/virt): 12345 / 67890 K
core:
  mempool(allocated/available): 1024 / 2048
  threads(count/active): 4 / 2
  context(count/sleep/pending): 10 / 8 / 1
  md_handler(count/pending): 5 / 0
  timer(count/pending): 7 / 1
sessions:
  starting: 1
  active: 100
  finishing: 2
pppoe:
  starting: 3
  active: 90
  delayed PADO: 4
  recv PADI: 1000
  drop PADI: 5
  sent PADO: 995
  recv PADR: 990
  recv PADR(dup): 2
  sent PADS: 988
  filtered: 1
radius(1, 10.0.0.1):
  state: active
  fail count: 0
  request count: 50
  queue length: 3
  auth sent: 500
  auth lost(total/5m/1m): 10 / 1 / 0
  auth avg time(5m/1m): 12.5 / 11.0
  acct sent: 480
  acct lost(total/5m/1m): 5 / 0 / 0
  acct avg time(5m/1m): 8.0 / 7.5
  interim sent: 200
  interim lost(total/5m/1m): 2 / 0 / 0
  interim avg time(5m/1m): 6.0 / 5.5
`

func wantEq(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestParseStatsMainAndCore(t *testing.T) {
	st, err := parseStats(sampleStat)
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	wantEq(t, "Uptime", st.Uptime, 138*86400+5*60+20) // 138d 00:05:20
	wantEq(t, "CPUPercent", st.CPUPercent, 1.50)
	wantEq(t, "MemRSS", st.MemRSS, 12345)
	wantEq(t, "MemVirt", st.MemVirt, 67890)

	wantEq(t, "Core.MempoolAllocated", st.Core.MempoolAllocated, 1024)
	wantEq(t, "Core.MempoolAvailable", st.Core.MempoolAvailable, 2048)
	wantEq(t, "Core.ThreadCount", st.Core.ThreadCount, 4)
	wantEq(t, "Core.ThreadActive", st.Core.ThreadActive, 2)
	wantEq(t, "Core.ContextCount", st.Core.ContextCount, 10)
	wantEq(t, "Core.ContextSleeping", st.Core.ContextSleeping, 8)
	wantEq(t, "Core.ContextPending", st.Core.ContextPending, 1)
	wantEq(t, "Core.MDHandlerCount", st.Core.MDHandlerCount, 5)
	wantEq(t, "Core.TimerCount", st.Core.TimerCount, 7)
	wantEq(t, "Core.TimerPending", st.Core.TimerPending, 1)
}

func TestParseStatsSessionsAndPPPoE(t *testing.T) {
	st, err := parseStats(sampleStat)
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	wantEq(t, "Sessions.Starting", st.Sessions.Starting, 1)
	wantEq(t, "Sessions.Active", st.Sessions.Active, 100)
	wantEq(t, "Sessions.Finishing", st.Sessions.Finishing, 2)

	wantEq(t, "PPPoE.Active", st.PPPoE.Active, 90)
	wantEq(t, "PPPoE.DelayedPADO", st.PPPoE.DelayedPADO, 4)
	wantEq(t, "PPPoE.RecvPADI", st.PPPoE.RecvPADI, 1000)
	// "recv PADR(dup)" must not be confused with "recv PADR".
	wantEq(t, "PPPoE.RecvPADR", st.PPPoE.RecvPADR, 990)
	wantEq(t, "PPPoE.RecvPADRDup", st.PPPoE.RecvPADRDup, 2)
	wantEq(t, "PPPoE.Filtered", st.PPPoE.Filtered, 1)
}

func TestParseStatsRadius(t *testing.T) {
	st, err := parseStats(sampleStat)
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	if len(st.RadiusServers) != 1 {
		t.Fatalf("RadiusServers = %d, want 1", len(st.RadiusServers))
	}
	rs, ok := st.RadiusServers["1"]
	if !ok {
		t.Fatalf("radius server id %q not parsed; got %v", "1", st.RadiusServers)
	}
	if rs.IP != "10.0.0.1" {
		t.Errorf("radius IP = %q, want 10.0.0.1", rs.IP)
	}
	if rs.State != "active" {
		t.Errorf("radius State = %q, want active", rs.State)
	}
	wantEq(t, "AuthSent", rs.AuthSent, 500)
	// auth lost(total/5m/1m): 10 / 1 / 0
	wantEq(t, "AuthLostTotal", rs.AuthLostTotal, 10)
	wantEq(t, "AuthLost5m", rs.AuthLost5m, 1)
	wantEq(t, "AuthLost1m", rs.AuthLost1m, 0)
	// auth avg time(5m/1m): 12.5 / 11.0
	wantEq(t, "AuthAvgTime5m", rs.AuthAvgTime5m, 12.5)
	wantEq(t, "AuthAvgTime1m", rs.AuthAvgTime1m, 11.0)
	wantEq(t, "AcctLostTotal", rs.AcctLostTotal, 5)
	wantEq(t, "InterimAvgTime1m", rs.InterimAvgTime1m, 5.5)
}

// TestParseStatsMultipleRadius verifies servers are keyed by id and a second
// server does not clobber the first.
func TestParseStatsMultipleRadius(t *testing.T) {
	in := `radius(1, 10.0.0.1):
  state: active
  auth sent: 5
radius(2, 10.0.0.2):
  state: failed
  auth sent: 9
`
	st, err := parseStats(in)
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	if len(st.RadiusServers) != 2 {
		t.Fatalf("RadiusServers = %d, want 2", len(st.RadiusServers))
	}
	if st.RadiusServers["1"].IP != "10.0.0.1" || st.RadiusServers["2"].IP != "10.0.0.2" {
		t.Errorf("server IPs not keyed correctly: %+v", st.RadiusServers)
	}
	if st.RadiusServers["2"].State != "failed" {
		t.Errorf("server 2 State = %q, want failed", st.RadiusServers["2"].State)
	}
}

func TestParseStatsEmpty(t *testing.T) {
	st, err := parseStats("")
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	if st == nil {
		t.Fatal("parseStats returned nil stats")
	}
	if len(st.RadiusServers) != 0 {
		t.Errorf("RadiusServers = %d, want 0", len(st.RadiusServers))
	}
}

// TestParseStatsMalformed verifies malformed numeric fields degrade to 0
// without aborting the parse or corrupting sibling fields. A bad sub-field in a
// "/"-delimited value becomes 0 while its valid neighbours still parse.
func TestParseStatsMalformed(t *testing.T) {
	in := `radius(1, 10.0.0.1):
  state: active
  auth sent: notanumber
  auth lost(total/5m/1m): bad / 1 / 0
`
	st, err := parseStats(in)
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	rs := st.RadiusServers["1"]
	wantEq(t, "AuthSent", rs.AuthSent, 0)           // unparseable scalar -> 0
	wantEq(t, "AuthLostTotal", rs.AuthLostTotal, 0) // bad sub-field -> 0
	wantEq(t, "AuthLost5m", rs.AuthLost5m, 1)       // neighbours still parse
	wantEq(t, "AuthLost1m", rs.AuthLost1m, 0)
}

func TestParseUptime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{"days and time", "138.00:05:20", 138*86400 + 5*60 + 20},
		{"zero", "0.00:00:00", 0},
		{"hours only", "1.02:00:00", 86400 + 2*3600},
		{"missing dot", "00:05:20", 0},
		{"bad days", "abc.00:05:20", 0},
		{"short time", "1.05:20", 0},
		{"empty", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseUptime(tt.in); got != tt.want {
				t.Errorf("parseUptime(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParsePercentage(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"1.50%", 1.50},
		{"0%", 0},
		{"42", 42}, // tolerant of a missing suffix
		{"bad%", 0},
	}
	for _, tt := range tests {
		if got := parsePercentage(tt.in); got != tt.want {
			t.Errorf("parsePercentage(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseMemory(t *testing.T) {
	st := &Stats{}
	parseMemory(st, "12345 / 67890 K")
	wantEq(t, "MemRSS", st.MemRSS, 12345)
	wantEq(t, "MemVirt", st.MemVirt, 67890)

	// Malformed input leaves the values untouched (not NaN).
	bad := &Stats{}
	parseMemory(bad, "garbage")
	if math.IsNaN(bad.MemRSS) || bad.MemRSS != 0 {
		t.Errorf("MemRSS after bad parse = %v, want 0", bad.MemRSS)
	}
}
