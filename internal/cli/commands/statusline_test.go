package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

// The status bar is rendered by tmux's #(), which captures stdout only. cobra's
// cmd.Print writes to stderr, so the command must write to stdout explicitly;
// guard that, or the status bar silently shows nothing.
func TestStatuslineCommandWritesToStdout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out, errb bytes.Buffer
	statuslineCmd.SetOut(&out)
	statuslineCmd.SetErr(&errb)
	if err := statuslineCmd.RunE(statuslineCmd, []string{"myrepo__claude"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "⚓ myrepo claude") {
		t.Fatalf("stdout = %q, want the status line", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", errb.String())
	}
}

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

func TestWaitingBadge(t *testing.T) {
	if got := waitingBadge(0); got != "" {
		t.Fatalf("zero should be empty, got %q", got)
	}
	if got := waitingBadge(-1); got != "" {
		t.Fatalf("negative should be empty, got %q", got)
	}
	got := waitingBadge(2)
	if !strings.Contains(got, "2 waiting") || !strings.Contains(got, "fg=yellow") {
		t.Fatalf("got %q", got)
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
