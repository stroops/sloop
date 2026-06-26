package commands

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stroops/sloop/internal/runner"
)

func TestFleetRowsFiltersAndSplits(t *testing.T) {
	in := []runner.TmuxSession{
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
	_ = RunPs(&b, []FleetRow{{Workspace: "svc", Tool: "claude", Name: "svc__claude", Windows: 1, Activity: time.Now()}})
	out := b.String()
	if !strings.Contains(out, "svc") || !strings.Contains(out, "claude") || !strings.Contains(out, "1 running") {
		t.Fatalf("populated output: %s", out)
	}
}

func TestJumpToFleetBounds(t *testing.T) {
	if err := jumpToFleet(nil, 1); err == nil {
		t.Fatal("expected error for empty fleet")
	}
}
