package commands

import (
	"strings"
	"testing"
)

func TestRunToolsListsClaude(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var b strings.Builder
	if err := RunTools(&b); err != nil {
		t.Fatalf("RunTools: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "claude") || !strings.Contains(out, "cursor") {
		t.Fatalf("tools output missing adapters:\n%s", out)
	}
}

func TestRunDoctorReportsTmuxAndMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var b strings.Builder
	if err := RunDoctor(&b); err != nil {
		t.Fatalf("RunDoctor: %v", err)
	}
	out := strings.ToLower(b.String())
	if !strings.Contains(out, "tmux") || !strings.Contains(out, "mode") {
		t.Fatalf("doctor output missing tmux/mode:\n%s", out)
	}
}
