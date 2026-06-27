package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunSkillNewCreatesAndLinks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // empty PATH → claude fallback (has .claude/skills target)
	if err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	path, linked, err := RunSkillNew(dir, "review")
	if err != nil {
		t.Fatalf("RunSkillNew: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("skill file not created: %v", err)
	}
	if filepath.Base(path) != "review.md" {
		t.Fatalf("want review.md, got %s", path)
	}
	// The skill is delivered: the tool's skills dir is now a symlink.
	fi, err := os.Lstat(filepath.Join(dir, ".claude", "skills"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected .claude/skills symlink after skill new: %v", err)
	}
	if len(linked) == 0 {
		t.Fatalf("expected the skill to be linked into a tool, got %v", linked)
	}
	// The new skill is visible through the symlink.
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "review.md")); err != nil {
		t.Fatalf("skill not visible via symlink: %v", err)
	}
	// Creating again should error.
	if _, _, err := RunSkillNew(dir, "review"); err == nil {
		t.Fatal("expected error creating duplicate skill")
	}
}
