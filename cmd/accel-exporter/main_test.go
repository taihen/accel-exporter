package main

import (
	"strings"
	"testing"
)

// TestVersionInfo pins the format of the build-info string emitted at startup
// and on the index page. ldflags overwrite these vars at release time; the test
// asserts the layout, not the default values.
func TestVersionInfo(t *testing.T) {
	version, commit, date = "1.2.3", "abc1234", "2026-06-19T00:00:00Z"
	got := versionInfo()
	for _, want := range []string{"1.2.3", "abc1234", "2026-06-19T00:00:00Z", "accel-exporter version"} {
		if !strings.Contains(got, want) {
			t.Errorf("versionInfo() = %q, missing %q", got, want)
		}
	}
}
