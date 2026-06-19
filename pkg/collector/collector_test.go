package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewAccelCollector(t *testing.T) {
	if c := NewAccelCollector("accel-cmd"); c == nil {
		t.Fatal("NewAccelCollector returned nil")
	}
}

// TestDescribeEmitsDescriptors guards that Describe reports the collector's
// fixed metrics, so prometheus.MustRegister sees a non-empty, conflict-free set.
func TestDescribeEmitsDescriptors(t *testing.T) {
	c := NewAccelCollector("accel-cmd")
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
	reg.MustRegister(NewAccelCollector("/nonexistent/accel-cmd-xyz"))

	vals := gather(t, reg)
	if up, ok := vals["accel_up"]; !ok || up != 0 {
		t.Errorf("accel_up = %v (present=%v), want 0", up, ok)
	}
	if f, ok := vals["accel_scrape_failures_total"]; !ok || f < 1 {
		t.Errorf("accel_scrape_failures_total = %v (present=%v), want >= 1", f, ok)
	}
}
