package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

func TestContextStateTransitions(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Name: "Claude Code", Context: adapter.ContextSpec{Mode: "pointer", File: "CLAUDE.md"}}
	if got := ContextState(root, m); got != "missing" {
		t.Fatalf("want missing, got %s", got)
	}
	if _, err := SyncContext(root, m); err != nil {
		t.Fatal(err)
	}
	if got := ContextState(root, m); got != "ok" {
		t.Fatalf("want ok, got %s", got)
	}
	if got := ContextState(root, adapter.Manifest{Context: adapter.ContextSpec{Mode: "native"}}); got != "native" {
		t.Fatalf("want native, got %s", got)
	}
}

func TestSkillsStateLinked(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	if got := SkillsState(root, sloopDir, m); got != "missing" {
		t.Fatalf("want missing, got %s", got)
	}
	if _, err := SyncSkills(root, sloopDir, m); err != nil {
		t.Fatal(err)
	}
	if got := SkillsState(root, sloopDir, m); got != "linked" {
		t.Fatalf("want linked, got %s", got)
	}
	if got := SkillsState(root, sloopDir, adapter.Manifest{}); got != "none" {
		t.Fatalf("want none, got %s", got)
	}
}
