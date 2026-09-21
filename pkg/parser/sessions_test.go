package parser

import (
	"context"
	"testing"
)

const sessionsHeader = " sid | username | rate-limit | uptime-raw | rx-bytes-raw | tx-bytes-raw | rx-pkts | tx-pkts\n" +
	"------+----------+------------+------------+--------------+--------------+---------+--------\n"

const (
	rowVDSL  = " sid-1 | user1#isp@vdsl | 102400/40960 | 122529 | 850000000 | 18000000000 | 900000 | 1200000\n"
	rowFTTH  = " sid-2 | user2#isp@ftth |  | 57903 | 100 | 200 | 3 | 4\n"
	rowPlain = " sid-3 | customer-a | 50000/50000 | 10 | 1 | 2 | 3 | 4\n"
)

func TestParseSessionsReadsAllColumns(t *testing.T) {
	sessions, errs := parseSessions(sessionsHeader + rowVDSL)
	if errs != 0 || len(sessions) != 1 {
		t.Fatalf("got %d sessions, %d errors", len(sessions), errs)
	}
	s := sessions[0]
	if s.SID != "sid-1" || s.Username != "user1#isp@vdsl" {
		t.Errorf("username wrong: %+v", s)
	}
	if s.RxBytes != 850000000 || s.TxBytes != 18000000000 || s.RxPackets != 900000 || s.TxPackets != 1200000 || s.Uptime != 122529 {
		t.Errorf("counter fields wrong: %+v", s)
	}
	if !s.HasRate || s.RateDownKbit != 102400 || s.RateUpKbit != 40960 {
		t.Errorf("rate wrong: %+v", s)
	}
}

func TestParseSessionsWithoutRateLimit(t *testing.T) {
	sessions, _ := parseSessions(sessionsHeader + rowFTTH)
	if len(sessions) != 1 || sessions[0].HasRate {
		t.Fatalf("unexpected: %+v", sessions)
	}
}

func TestParseSessionsSeveralRows(t *testing.T) {
	sessions, errs := parseSessions(sessionsHeader + rowVDSL + rowFTTH + rowPlain)
	if len(sessions) != 3 || errs != 0 {
		t.Fatalf("got %d sessions, %d errors", len(sessions), errs)
	}
}

func TestParseSessionsEmptyOutput(t *testing.T) {
	for _, in := range []string{"", sessionsHeader} {
		if sessions, errs := parseSessions(in); len(sessions) != 0 || errs != 0 {
			t.Errorf("input %q: got %d sessions, %d errors", in, len(sessions), errs)
		}
	}
}

func TestParseSessionsCountsMalformedRows(t *testing.T) {
	bad := " broken | row\n not-a-table-line\n s | l | | notanumber | 1 | 1 | 1 | 1\n"
	sessions, errs := parseSessions(sessionsHeader + rowVDSL + bad)
	if len(sessions) != 1 || errs != 3 {
		t.Fatalf("got %d sessions, %d errors, want 1 and 3", len(sessions), errs)
	}
}

func TestRealmOf(t *testing.T) {
	cases := map[string]string{
		"user1#isp@vdsl":   "isp@vdsl",
		"47073#other@vdsl": "other@vdsl",
		"customer-a":       "none",
		"trailing#":        "none",
	}
	for user, want := range cases {
		if got := RealmOf(user); got != want {
			t.Errorf("RealmOf(%q) = %q, want %q", user, got, want)
		}
	}
}

// The same username can have several live sessions (accel-ppp allows that
// unless single-session is set); they are told apart by sid.
func TestParseSessionsKeepsConcurrentSessionsOfOneUser(t *testing.T) {
	second := " sid-9 | user1#isp@vdsl | 1/1 | 5 | 9 | 9 | 9 | 9\n"
	sessions, errs := parseSessions(sessionsHeader + rowVDSL + second)
	if errs != 0 || len(sessions) != 2 {
		t.Fatalf("got %d sessions, %d errors, want 2 and 0", len(sessions), errs)
	}
	if sessions[0].SID == sessions[1].SID {
		t.Errorf("sessions not distinguished by sid: %+v", sessions)
	}
}

func TestCollectSessionsRunsShowSessionsWithRawColumns(t *testing.T) {
	path := fakeAccelCmd(t, "[ \"$1 $2\" = \"show sessions\" ] || exit 1\n"+
		"case \"$3\" in *sid*username*rx-bytes-raw*tx-bytes-raw*) ;; *) exit 2;; esac\n"+
		"case \"$3\" in *,ip*|ip,*|*ifname*) exit 3;; esac\n"+
		"cat <<'EOF'\n"+sessionsHeader+rowVDSL+"EOF")
	sessions, errs, err := CollectSessions(context.Background(), path)
	if err != nil || errs != 0 || len(sessions) != 1 {
		t.Fatalf("got %v sessions, %d errors, err=%v", sessions, errs, err)
	}
}

func TestCollectSessionsFailure(t *testing.T) {
	if _, _, err := CollectSessions(context.Background(), "/nonexistent/accel-cmd-xyz"); err == nil {
		t.Fatal("want an error for a missing accel-cmd")
	}
}
