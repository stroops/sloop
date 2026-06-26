package runner

import "testing"

func TestParseSessions(t *testing.T) {
	raw := "sloop__claude\t1\t2\t1719421234\nbackend__cursor\t0\t1\t1719420000\n"
	got := ParseSessions(raw)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].Name != "sloop__claude" || !got[0].Attached || got[0].Windows != 2 {
		t.Fatalf("row0 = %+v", got[0])
	}
	if got[1].Attached {
		t.Fatalf("row1 should be detached: %+v", got[1])
	}
}

func TestParseSessionsSkipsMalformed(t *testing.T) {
	if got := ParseSessions("garbage\n\n"); len(got) != 0 {
		t.Fatalf("want 0, got %+v", got)
	}
}

func TestBuildTmuxSwitchArgs(t *testing.T) {
	got := BuildTmuxSwitchArgs("ws__claude")
	if len(got) != 3 || got[0] != "switch-client" || got[2] != "ws__claude" {
		t.Fatalf("got %v", got)
	}
}
