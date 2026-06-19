package collector

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const sampleStat = `uptime: 138.00:05:20
cpu: 1.50%
mem(rss/virt): 12345 / 67890 K
pppoe:
  active: 90
  recv PADI: 1000
radius(1, 10.0.0.1):
  state: active
  auth sent: 500
`

// fakeCollector returns a collector backed by an executable shell script that
// prints sampleStat, plus the script path. Skips on windows.
func fakeCollector(t *testing.T) *AccelCollector {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake not supported on windows")
	}
	path := filepath.Join(t.TempDir(), "accel-cmd")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat <<'EOF'\n"+sampleStat+"EOF\n"), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	return NewAccelCollector(path, time.Second)
}

func TestNewAccelCollector(t *testing.T) {
	if c := NewAccelCollector("accel-cmd", time.Second); c == nil {
		t.Fatal("NewAccelCollector returned nil")
	}
}

// TestNewAccelCollectorDefaultsTimeout guards the non-positive-timeout fallback.
func TestNewAccelCollectorDefaultsTimeout(t *testing.T) {
	if c := NewAccelCollector("accel-cmd", 0); c.timeout != DefaultScrapeTimeout {
		t.Errorf("timeout = %v, want %v", c.timeout, DefaultScrapeTimeout)
	}
}

// TestDescribeEmitsDescriptors guards that Describe reports the collector's
// fixed metrics, so prometheus.MustRegister sees a non-empty, conflict-free set.
func TestDescribeEmitsDescriptors(t *testing.T) {
	c := NewAccelCollector("accel-cmd", time.Second)
	ch := make(chan *prometheus.Desc, 256)
	c.Describe(ch)
	close(ch)
	if len(ch) == 0 {
		t.Fatal("Describe emitted no descriptors")
	}
}

// gather collects the registry and returns metric values keyed by family name.
// Families with labels (e.g. radius_*) are skipped; the scrape-failure path
// under test emits only the unlabelled accel_up and accel_scrape_failures_total.
func gather(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	out := make(map[string]float64)
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			if len(m.GetLabel()) != 0 {
				continue
			}
			switch {
			case m.GetGauge() != nil:
				out[mf.GetName()] = m.GetGauge().GetValue()
			case m.GetCounter() != nil:
				out[mf.GetName()] = m.GetCounter().GetValue()
			}
		}
	}
	return out
}

// TestCollectScrapeFailure drives the error path: an accel-cmd path that cannot
// execute must publish accel_up=0 and bump accel_scrape_failures_total, never
// crash the scrape. This is the only collector behaviour exercisable without a
// live accel-ppp, and the one that matters most for alerting.
func TestCollectScrapeFailure(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(NewAccelCollector("/nonexistent/accel-cmd-xyz", time.Second))

	vals := gather(t, reg)
	if up, ok := vals["accel_up"]; !ok || up != 0 {
		t.Errorf("accel_up = %v (present=%v), want 0", up, ok)
	}
	if f, ok := vals["accel_scrape_failures_total"]; !ok || f < 1 {
		t.Errorf("accel_scrape_failures_total = %v (present=%v), want >= 1", f, ok)
	}
}

// TestCollectSuccess drives the happy path against a fake accel-cmd: accel_up=1,
// counters carry Counter type (not Gauge), and labelled RADIUS series appear.
func TestCollectSuccess(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(fakeCollector(t))

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	byName := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		byName[mf.GetName()] = mf
	}

	if up := byName["accel_up"]; up == nil || up.GetMetric()[0].GetGauge().GetValue() != 1 {
		t.Errorf("accel_up missing or != 1: %v", up)
	}
	// recv_padi_total must be a Counter now, not a Gauge.
	if padi := byName["accel_pppoe_recv_padi_total"]; padi == nil || padi.GetType() != dto.MetricType_COUNTER {
		t.Errorf("accel_pppoe_recv_padi_total type = %v, want COUNTER", padi.GetType())
	}
	// labelled RADIUS series present with correct labels.
	state := byName["accel_radius_state"]
	if state == nil || len(state.GetMetric()) != 1 {
		t.Fatalf("accel_radius_state missing: %v", state)
	}
	labels := map[string]string{}
	for _, l := range state.GetMetric()[0].GetLabel() {
		labels[l.GetName()] = l.GetValue()
	}
	if labels["server_id"] != "1" || labels["server_ip"] != "10.0.0.1" {
		t.Errorf("radius labels = %v, want server_id=1 server_ip=10.0.0.1", labels)
	}
}

// TestCollectConcurrent runs many overlapping scrapes; with the stateless
// const-metric design this must be race-free (run with -race) and never panic.
func TestCollectConcurrent(t *testing.T) {
	c := fakeCollector(t)
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(c)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if _, err := reg.Gather(); err != nil {
				t.Errorf("Gather: %v", err)
			}
		})
	}
	wg.Wait()
}
