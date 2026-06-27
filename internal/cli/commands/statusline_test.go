package commands

import (
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

func TestRenderStatusline(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no markers; no live session → unknown
	out := renderStatusline("myrepo__claude")
	if !strings.Contains(out, "⚓ myrepo claude") {
		t.Fatalf("statusline = %q", out)
	}
	if renderStatusline("") != "" {
		t.Fatal("empty session → empty")
	}
}

func TestTmuxStatusLabel(t *testing.T) {
	w := tmuxStatusLabel(tmux.StatusWaiting)
	if !strings.Contains(w, "waiting") || !strings.Contains(w, "fg=yellow") {
		t.Fatalf("waiting label = %q", w)
	}
	if !strings.Contains(tmuxStatusLabel(tmux.StatusWorking), "working") {
		t.Fatalf("working label")
	}
}
