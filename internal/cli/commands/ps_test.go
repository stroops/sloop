package commands

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/tmux"
)

func TestFleetRowsFiltersAndSplits(t *testing.T) {
	in := []tmux.Session{
		{Name: "my_app__claude", Attached: true, Windows: 2},
		{Name: "backend__cursor", Windows: 1},
		{Name: "random", Windows: 1}, // not a sloop session
	}
	rows := fleetRows(in, nil)
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

func TestSplitSession(t *testing.T) {
	m := map[string]adapter.Manifest{"claude": {}, "cursor": {}}
	cases := []struct {
		name                    string
		in                      string
		manifests               map[string]adapter.Manifest
		wantWs, wantTool, wInst string
	}{
		{"legacy two-segment", "repo__claude", m, "repo", "claude", ""},
		{"named instance", "repo__claude__sec", m, "repo", "claude", "sec"},
		{"numeric instance", "repo__claude__2", m, "repo", "claude", "2"},
		{"workspace with __", "a__b__claude", m, "a__b", "claude", ""},
		{"unknown tool falls back", "repo__weird", nil, "repo", "weird", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ws, tool, inst := splitSession(c.in, c.manifests)
			if ws != c.wantWs || tool != c.wantTool || inst != c.wInst {
				t.Fatalf("got (%q,%q,%q) want (%q,%q,%q)", ws, tool, inst, c.wantWs, c.wantTool, c.wInst)
			}
		})
	}
}

func TestToolNameWithInstance(t *testing.T) {
	if got := (FleetRow{Tool: "claude", Instance: "sec"}).toolName(); got != "claude·sec" {
		t.Fatalf("got %q", got)
	}
	if got := (FleetRow{Tool: "claude"}).toolName(); got != "claude" {
		t.Fatalf("got %q", got)
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

func TestHumanizeDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "now"},
		{30 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{3 * 24 * time.Hour, "3d"},
		{-time.Minute, "now"}, // already past (e.g. a rate limit reset that just happened)
	}
	for _, c := range cases {
		if got := humanizeDuration(c.d); got != c.want {
			t.Fatalf("humanizeDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestModelCtxPrefix(t *testing.T) {
	if got := modelCtxPrefix(FleetRow{}); got != "" {
		t.Fatalf("no model/ctx → empty, got %q", got)
	}
	if got := modelCtxPrefix(FleetRow{Model: "Opus"}); got != "Opus" {
		t.Fatalf("model only = %q", got)
	}
	if got := modelCtxPrefix(FleetRow{CtxPct: 45}); got != "ctx 45%" {
		t.Fatalf("ctx only = %q", got)
	}
	if got := modelCtxPrefix(FleetRow{Model: "Opus", CtxPct: 45}); got != "Opus · ctx 45%" {
		t.Fatalf("both = %q", got)
	}
}

// The model/ctx prefix must lead the bottom line, with the glance/prompt
// still present after it — this is what makes model+context visible in the
// fleet view (sloop ps), not just the tmux status bar.
func TestBottomLineCarriesModelAndCtx(t *testing.T) {
	r := FleetRow{Model: "Opus", CtxPct: 45, Glance: "fixing the parser bug"}
	got := bottomLine(r, 80)
	if !strings.Contains(got, "Opus · ctx 45%") {
		t.Fatalf("missing model/ctx prefix: %q", got)
	}
	if !strings.Contains(got, "fixing the parser bug") {
		t.Fatalf("missing glance: %q", got)
	}

	// No model/ctx known → falls back to the plain glance, unchanged.
	plain := FleetRow{Glance: "fixing the parser bug"}
	if got := bottomLine(plain, 80); got != "fixing the parser bug" {
		t.Fatalf("plain glance = %q", got)
	}

	// A waiting row's prompt still comes after the prefix.
	waiting := FleetRow{Model: "Opus", CtxPct: 45, Status: tmux.StatusWaiting, Prompt: "Which approach?"}
	got = bottomLine(waiting, 80)
	if !strings.Contains(got, "Opus · ctx 45%") || !strings.Contains(got, "Which approach?") {
		t.Fatalf("waiting bottom line = %q", got)
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
	// Non-running workspace paths must exist on disk (stale/missing paths are filtered).
	infraDir := t.TempDir()
	docsDir := t.TempDir()
	paths := map[string]string{"api": "/a", "web": "/w", "infra": infraDir, "docs": docsDir}
	got := notRunningWorkspaces(rows, paths)
	if len(got) != 2 || got[0] != "docs" || got[1] != "infra" {
		t.Fatalf("got %v, want [docs infra]", got)
	}
}

func TestNotRunningWorkspacesSkipsStale(t *testing.T) {
	rows := []FleetRow{{Workspace: "api"}}
	infraDir := t.TempDir()
	paths := map[string]string{"api": "/a", "infra": infraDir, "stale": "/nonexistent/path/xyz"}
	got := notRunningWorkspaces(rows, paths)
	if len(got) != 1 || got[0] != "infra" {
		t.Fatalf("got %v, want [infra] (stale should be filtered)", got)
	}
}

func TestRunPsAllShowsNotRunning(t *testing.T) {
	var b bytes.Buffer
	rows := []FleetRow{{Workspace: "api", Tool: "claude", Name: "api__claude", Activity: time.Now()}}
	infraDir := t.TempDir()
	paths := map[string]string{"api": "/a", "infra": infraDir}
	_ = runPsAll(&b, rows, paths, nil)
	out := b.String()
	if !strings.Contains(out, "Known workspaces (not running)") ||
		!strings.Contains(out, "infra") || !strings.Contains(out, infraDir) {
		t.Fatalf("missing not-running section: %s", out)
	}
}
