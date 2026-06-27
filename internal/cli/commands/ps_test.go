package commands

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stroops/sloop/internal/tmux"
)

func TestFleetRowsFiltersAndSplits(t *testing.T) {
	in := []tmux.Session{
		{Name: "my_app__claude", Attached: true, Windows: 2},
		{Name: "backend__cursor", Windows: 1},
		{Name: "random", Windows: 1}, // not a sloop session
	}
	rows := fleetRows(in)
	if len(rows) != 2 {
		t.Fatalf("want 2 sloop rows, got %d (%+v)", len(rows), rows)
	}
	// sorted by workspace: backend before my_app
	if rows[0].Workspace != "backend" || rows[0].Tool != "cursor" {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1].Workspace != "my_app" || rows[1].Tool != "claude" || !rows[1].Attached {
		t.Fatalf("row1 = %+v", rows[1])
	}
}

func TestRunPsEmptyAndPopulated(t *testing.T) {
	var b bytes.Buffer
	_ = RunPs(&b, nil)
	if !strings.Contains(b.String(), "No running AI sessions") {
		t.Fatalf("empty output: %s", b.String())
	}
	b.Reset()
	_ = RunPs(&b, []FleetRow{{Workspace: "svc", Tool: "claude", Name: "svc__claude", Activity: time.Now(), Glance: "Waiting for your approval"}})
	out := b.String()
	if !strings.Contains(out, "svc") || !strings.Contains(out, "claude") || !strings.Contains(out, "1 running") {
		t.Fatalf("populated output: %s", out)
	}
	if !strings.Contains(out, "Waiting for your approval") {
		t.Fatalf("glance line missing: %s", out)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("got %q", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcd…" {
		t.Fatalf("got %q", got)
	}
}

func TestJumpToFleetBounds(t *testing.T) {
	if err := jumpToFleet(nil, 1); err == nil {
		t.Fatal("expected error for empty fleet")
	}
}

func TestFilterWaitingAndNewlyWaiting(t *testing.T) {
	waiting := FleetRow{Name: "a__claude", Status: tmux.StatusWaiting}
	working := FleetRow{Name: "b__cursor", Status: tmux.StatusWorking}
	idle := FleetRow{Name: "c__claude", Status: tmux.StatusIdle}

	got := filterWaiting([]FleetRow{waiting, working, idle})
	if len(got) != 1 || got[0].Name != "a__claude" {
		t.Fatalf("filterWaiting = %+v", got)
	}

	// b__cursor flips from working to waiting → newly waiting; a__claude was
	// already waiting → not reported again.
	prev := []FleetRow{waiting, working, idle}
	curr := []FleetRow{
		waiting,
		{Name: "b__cursor", Status: tmux.StatusWaiting},
		idle,
	}
	nw := newlyWaiting(prev, curr)
	if len(nw) != 1 || nw[0] != "b__cursor" {
		t.Fatalf("newlyWaiting = %v", nw)
	}
	if n := newlyWaiting(curr, curr); len(n) != 0 {
		t.Fatalf("stable snapshot should yield none, got %v", n)
	}
}

func TestNotRunningWorkspaces(t *testing.T) {
	rows := []FleetRow{{Workspace: "api"}, {Workspace: "web"}}
	paths := map[string]string{"api": "/a", "web": "/w", "infra": "/i", "docs": "/d"}
	got := notRunningWorkspaces(rows, paths)
	if len(got) != 2 || got[0] != "docs" || got[1] != "infra" {
		t.Fatalf("got %v, want [docs infra]", got)
	}
}

func TestRunPsAllShowsNotRunning(t *testing.T) {
	var b bytes.Buffer
	rows := []FleetRow{{Workspace: "api", Tool: "claude", Name: "api__claude", Activity: time.Now()}}
	paths := map[string]string{"api": "/a", "infra": "/srv/infra"}
	_ = runPsAll(&b, rows, paths, nil)
	out := b.String()
	if !strings.Contains(out, "Known workspaces (not running)") ||
		!strings.Contains(out, "infra") || !strings.Contains(out, "/srv/infra") {
		t.Fatalf("missing not-running section: %s", out)
	}
}
