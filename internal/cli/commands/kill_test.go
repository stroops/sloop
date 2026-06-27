package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

func sampleRows() []FleetRow {
	return []FleetRow{
		{Workspace: "api", Tool: "claude", Name: "api__claude", Status: tmux.StatusWaiting},
		{Workspace: "web", Tool: "cursor", Name: "web__cursor", Status: tmux.StatusIdle},
	}
}

func TestTargetsToKill(t *testing.T) {
	rows := sampleRows()

	if v, err := targetsToKill(rows, nil, true, false); err != nil || len(v) != 2 {
		t.Fatalf("--all: %v / %d", err, len(v))
	}
	if v, err := targetsToKill(rows, nil, false, true); err != nil || len(v) != 1 || v[0].Name != "api__claude" {
		t.Fatalf("--waiting: %v / %+v", err, v)
	}
	if v, err := targetsToKill(rows, []string{"2"}, false, false); err != nil || v[0].Name != "web__cursor" {
		t.Fatalf("by number: %v / %+v", err, v)
	}
	if _, err := targetsToKill(rows, nil, false, false); err == nil {
		t.Fatal("expected error with no target and no flags")
	}
	if _, err := targetsToKill(rows, []string{"nope"}, false, false); err == nil {
		t.Fatal("expected unknown-target error")
	}
}

func TestConfirm(t *testing.T) {
	for in, want := range map[string]bool{"y\n": true, "yes\n": true, "Y\n": true, "n\n": false, "\n": false, "nope\n": false} {
		var b bytes.Buffer
		if got := confirm(&b, strings.NewReader(in), "ok? "); got != want {
			t.Fatalf("confirm(%q) = %v, want %v", in, got, want)
		}
	}
}
