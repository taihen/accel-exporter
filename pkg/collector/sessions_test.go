package collector

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const sampleSessions = ` sid | username | rate-limit | uptime-raw | rx-bytes-raw | tx-bytes-raw | rx-pkts | tx-pkts
------+----------+------------+--------------+--------------+---------+--------
 sid-1 | user1#isp@vdsl | 102400/40960 | 122529 | 850000000 | 18000000000 | 900000 | 1200000
 sid-2 | user2#isp@ftth |  | 57903 | 100 | 200 | 3 | 4
`

// fakeSessionsCollector returns a collector backed by a fake accel-cmd that
// answers "show sessions" with sessionsOutput (or fails when it is empty) and
// everything else with the normal "show stat" sample.
func fakeSessionsCollector(t *testing.T, sessionsOutput string, opts ...Option) *AccelCollector {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake not supported on windows")
	}
	sessionsBranch := "exit 1"
	if sessionsOutput != "" {
		sessionsBranch = "cat <<'EOF'\n" + sessionsOutput + "EOF"
	}
	script := "#!/bin/sh\ncase \"$1 $2\" in\n\"show sessions\")\n" + sessionsBranch + "\n;;\n*)\ncat <<'EOF'\n" + sampleStat + "EOF\n;;\nesac\n"
	path := filepath.Join(t.TempDir(), "accel-cmd")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	return NewAccelCollector(path, time.Second, opts...)
}

func gatherFamilies(t *testing.T, c *AccelCollector) map[string]*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	out := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

func labelsOf(m *dto.Metric) map[string]string {
	out := map[string]string{}
	for _, l := range m.GetLabel() {
		out[l.GetName()] = l.GetValue()
	}
	return out
}

func findSeries(t *testing.T, mf *dto.MetricFamily, want map[string]string) *dto.Metric {
	t.Helper()
	if mf == nil {
		t.Fatalf("metric family missing")
	}
	for _, m := range mf.GetMetric() {
		got := labelsOf(m)
		match := true
		for k, v := range want {
			if got[k] != v {
				match = false
			}
		}
		if match {
			return m
		}
	}
	t.Fatalf("no series with labels %v in %s", want, mf.GetName())
	return nil
}

func TestSessionsAreOffByDefault(t *testing.T) {
	fams := gatherFamilies(t, fakeSessionsCollector(t, sampleSessions))
	for name := range fams {
		if strings.HasPrefix(name, "accel_session_") {
			t.Errorf("unexpected per-session family %s without WithSessions()", name)
		}
	}
}

func TestSessionCountersAreLabelledBySidUsernameAndRealm(t *testing.T) {
	fams := gatherFamilies(t, fakeSessionsCollector(t, sampleSessions, WithSessions()))

	user := map[string]string{"sid": "sid-1", "username": "user1#isp@vdsl", "realm": "isp@vdsl"}
	cases := map[string]float64{
		"accel_session_rx_bytes_total":   850000000,
		"accel_session_tx_bytes_total":   18000000000,
		"accel_session_rx_packets_total": 900000,
		"accel_session_tx_packets_total": 1200000,
	}
	for name, want := range cases {
		m := findSeries(t, fams[name], user)
		if m.GetCounter() == nil || m.GetCounter().GetValue() != want {
			t.Errorf("%s = %v, want counter %v", name, m, want)
		}
	}
	if m := findSeries(t, fams["accel_session_uptime_seconds"], user); m.GetGauge().GetValue() != 122529 {
		t.Errorf("uptime = %v, want 122529", m)
	}
}

func TestSessionLabelsNeverIncludeAddressesOrInterfaces(t *testing.T) {
	fams := gatherFamilies(t, fakeSessionsCollector(t, sampleSessions, WithSessions()))
	allowed := map[string]bool{"sid": true, "username": true, "realm": true}
	for name, mf := range fams {
		if !strings.HasPrefix(name, "accel_session_") {
			continue
		}
		for _, m := range mf.GetMetric() {
			for label := range labelsOf(m) {
				if !allowed[label] {
					t.Errorf("%s has unexpected label %q", name, label)
				}
			}
		}
	}
	if _, ok := fams["accel_session_info"]; ok {
		t.Error("accel_session_info must not exist")
	}
}

func TestSessionRateLimitsUseRxTxNamesAndBaseUnits(t *testing.T) {
	fams := gatherFamilies(t, fakeSessionsCollector(t, sampleSessions, WithSessions()))
	user := map[string]string{"username": "user1#isp@vdsl", "realm": "isp@vdsl"}

	// accel-ppp reports "down/up" in Kbit/s: 102400/40960. down = sent to the
	// subscriber (tx), up = received from the subscriber (rx); exported in
	// bytes per second, like every other unit in this collector.
	tx := findSeries(t, fams["accel_session_tx_rate_limit_bytes_per_second"], user)
	rx := findSeries(t, fams["accel_session_rx_rate_limit_bytes_per_second"], user)
	if tx.GetGauge().GetValue() != 12800000 {
		t.Errorf("tx rate limit = %v, want 12800000 (102400 kbit/s)", tx)
	}
	if rx.GetGauge().GetValue() != 5120000 {
		t.Errorf("rx rate limit = %v, want 5120000 (40960 kbit/s)", rx)
	}
	if _, ok := fams["accel_session_rate_limit_kbit"]; ok {
		t.Error("the direction-labelled accel_session_rate_limit_kbit must not exist")
	}
}

func TestUnshapedSessionsHaveNoRateLimitSeries(t *testing.T) {
	fams := gatherFamilies(t, fakeSessionsCollector(t, sampleSessions, WithSessions()))
	for _, name := range []string{"accel_session_rx_rate_limit_bytes_per_second", "accel_session_tx_rate_limit_bytes_per_second"} {
		for _, m := range fams[name].GetMetric() {
			if labelsOf(m)["username"] == "user2#isp@ftth" {
				t.Errorf("%s: unshaped session must not have a series: %v", name, m)
			}
		}
	}
}

func TestSessionsFailureKeepsAccelUpAndDropsSessionSeries(t *testing.T) {
	fams := gatherFamilies(t, fakeSessionsCollector(t, "", WithSessions()))
	if up := fams["accel_up"]; up == nil || up.GetMetric()[0].GetGauge().GetValue() != 1 {
		t.Errorf("accel_up must stay 1 when only show sessions fails: %v", up)
	}
	if _, ok := fams["accel_session_rx_bytes_total"]; ok {
		t.Error("session series must be dropped when show sessions fails")
	}
}

func TestSessionsNoSessionsEmitsNoSeries(t *testing.T) {
	header := strings.SplitAfter(sampleSessions, "\n")[:2]
	fams := gatherFamilies(t, fakeSessionsCollector(t, strings.Join(header, ""), WithSessions()))
	if mf, ok := fams["accel_session_rx_bytes_total"]; ok && len(mf.GetMetric()) != 0 {
		t.Errorf("want no per-session series, got %v", mf)
	}
}

// One username with two live sessions (accel-ppp's default, unless
// single-session is set) must yield one series per session, none dropped.
func TestConcurrentSessionsOfOneUserAreBothExported(t *testing.T) {
	second := sampleSessions + " sid-9 | user1#isp@vdsl | 1/1 | 5 | 9 | 9 | 9 | 9\n"
	fams := gatherFamilies(t, fakeSessionsCollector(t, second, WithSessions()))

	mf := fams["accel_session_tx_bytes_total"]
	if len(mf.GetMetric()) != 3 {
		t.Fatalf("want 3 series (2 sessions of user1 + user2), got %v", mf.GetMetric())
	}
	older := findSeries(t, mf, map[string]string{"sid": "sid-1"})
	newer := findSeries(t, mf, map[string]string{"sid": "sid-9"})
	if older.GetCounter().GetValue() != 18000000000 || newer.GetCounter().GetValue() != 9 {
		t.Errorf("per-session values mixed up: older=%v newer=%v", older, newer)
	}
	if _, ok := fams["accel_session_duplicates_dropped"]; ok {
		t.Error("accel_session_duplicates_dropped must not exist: nothing is dropped any more")
	}
}

// slowCollector returns a collector whose fake accel-cmd takes 700ms per call,
// against a 1s budget: two sequential calls fit only if each gets its own
// full timeout.
func slowCollector(t *testing.T, opts ...Option) *AccelCollector {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake not supported on windows")
	}
	script := "#!/bin/sh\nsleep 0.7\ncase \"$1 $2\" in\n\"show sessions\")\ncat <<'EOF'\n" + sampleSessions + "EOF\n;;\n*)\ncat <<'EOF'\n" + sampleStat + "EOF\n;;\nesac\n"
	path := filepath.Join(t.TempDir(), "accel-cmd")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	return NewAccelCollector(path, time.Second, opts...)
}

// TestScrapeSharesOneDeadlineAcrossCommands guards the HTTP WriteTimeout
// budget: -accel-cmd.timeout bounds the whole scrape, not each command.
func TestScrapeSharesOneDeadlineAcrossCommands(t *testing.T) {
	start := time.Now()
	fams := gatherFamilies(t, slowCollector(t, WithSessions()))
	elapsed := time.Since(start)

	if up := fams["accel_up"]; up == nil || up.GetMetric()[0].GetGauge().GetValue() != 1 {
		t.Errorf("accel_up = %v, want 1: show stat fits the budget", up)
	}
	if _, ok := fams["accel_session_rx_bytes_total"]; ok {
		t.Error("show sessions ran past the shared 1s deadline, its series must be dropped")
	}
	// The budget plus runAccelCmd's 2s WaitDelay (a killed script's child can
	// hold the pipe open that long) must stay inside the server's WriteTimeout
	// of budget+10s. Without a shared deadline the two calls would take ~1.4s
	// and show sessions would succeed, which the check above already catches.
	if limit := time.Second + 2*time.Second + 500*time.Millisecond; elapsed > limit {
		t.Errorf("scrape took %v, want at most %v", elapsed, limit)
	}
}

func TestParseErrorsAreReported(t *testing.T) {
	bad := sampleSessions + " broken | row\n"
	fams := gatherFamilies(t, fakeSessionsCollector(t, bad, WithSessions()))
	if e := fams["accel_session_parse_errors"]; e == nil || e.GetMetric()[0].GetGauge().GetValue() != 1 {
		t.Errorf("accel_session_parse_errors = %v, want 1", e)
	}
}
