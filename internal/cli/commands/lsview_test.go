package commands

import (
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

func TestAgentsInline(t *testing.T) {
	// Color is off in tests (stdout isn't a tty), so dots render plain.
	if got := agentsInline(nil); got != "-" {
		t.Errorf("empty = %q, want -", got)
	}
	rows := []FleetRow{
		{Tool: "claude", Display: "Claude Code", Status: tmux.StatusWaiting},
		{Tool: "cursor", Display: "Cursor", Status: tmux.StatusWorking},
	}
	want := "● Claude Code  ● Cursor"
	if got := agentsInline(rows); got != want {
		t.Errorf("agentsInline = %q, want %q", got, want)
	}
}
