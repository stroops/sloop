package commands

import "testing"

func TestAttachArgsBuilt(t *testing.T) {
	// RunAttach builds the tmux attach args for the given session name.
	if got := attachArgs("backend__claude"); got != "attach -t backend__claude" {
		t.Fatalf("unexpected: %s", got)
	}
}
