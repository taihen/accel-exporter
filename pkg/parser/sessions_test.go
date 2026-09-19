package parser

import "testing"

const sessionsHeader = " username | rate-limit | uptime-raw | rx-bytes-raw | tx-bytes-raw | rx-pkts | tx-pkts\n" +
	"----------+------------+------------+--------------+--------------+---------+--------\n"

const (
	rowVDSL  = " user1#isp@vdsl | 102400/40960 | 122529 | 850000000 | 18000000000 | 900000 | 1200000\n"
	rowFTTH  = " user2#isp@ftth |  | 57903 | 100 | 200 | 3 | 4\n"
	rowPlain = " customer-a | 50000/50000 | 10 | 1 | 2 | 3 | 4\n"
)

func TestParseSessionsReadsAllColumns(t *testing.T) {
	sessions, errs := parseSessions(sessionsHeader + rowVDSL)
	if errs != 0 || len(sessions) != 1 {
		t.Fatalf("got %d sessions, %d errors", len(sessions), errs)
	}
	s := sessions[0]
	if s.Username != "user1#isp@vdsl" {
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
	bad := " broken | row\n not-a-table-line\n l | | notanumber | 1 | 1 | 1 | 1\n"
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

func TestDedupeSessionsKeepsNewest(t *testing.T) {
	older := Session{Username: "dup#r", RxBytes: 1, Uptime: 5000}
	newer := Session{Username: "dup#r", RxBytes: 9, Uptime: 5}
	other := Session{Username: "other", Uptime: 1}
	got, dropped := DedupeSessions([]Session{older, newer, other})
	if dropped != 1 || len(got) != 2 {
		t.Fatalf("got %d sessions, %d dropped", len(got), dropped)
	}
	for _, s := range got {
		if s.Username == "dup#r" && s.RxBytes != 9 {
			t.Errorf("kept the older session: %+v", s)
		}
	}
}

func TestCollectSessionsRunsShowSessionsWithRawColumns(t *testing.T) {
	path := fakeAccelCmd(t, "[ \"$1 $2\" = \"show sessions\" ] || exit 1\n"+
		"case \"$3\" in *username*rx-bytes-raw*tx-bytes-raw*) ;; *) exit 2;; esac\n"+
		"case \"$3\" in *,ip*|ip,*|*ifname*) exit 3;; esac\n"+
		"cat <<'EOF'\n"+sessionsHeader+rowVDSL+"EOF")
	sessions, errs, err := CollectSessions(path, 0)
	if err != nil || errs != 0 || len(sessions) != 1 {
		t.Fatalf("got %v sessions, %d errors, err=%v", sessions, errs, err)
	}
}

func TestCollectSessionsFailure(t *testing.T) {
	if _, _, err := CollectSessions("/nonexistent/accel-cmd-xyz", 0); err == nil {
		t.Fatal("want an error for a missing accel-cmd")
	}
}
