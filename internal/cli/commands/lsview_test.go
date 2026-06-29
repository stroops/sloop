package commands

import (
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

func TestAbbrevHome(t *testing.T) {
	t.Setenv("HOME", "/Users/me")
	cases := []struct{ in, want string }{
		{"/Users/me", "~"},
		{"/Users/me/code/api", "~/code/api"},
		{"/opt/other", "/opt/other"},
		{"/Users/melon/x", "/Users/melon/x"}, // prefix but not a path boundary
	}
	for _, c := range cases {
		if got := abbrevHome(c.in); got != c.want {
			t.Errorf("abbrevHome(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

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
