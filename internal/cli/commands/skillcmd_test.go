package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunSkillNewCreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	path, err := RunSkillNew(dir, "review")
	if err != nil {
		t.Fatalf("RunSkillNew: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("skill file not created: %v", err)
	}
	if filepath.Base(path) != "review.md" {
		t.Fatalf("want review.md, got %s", path)
	}
	// Creating again should error.
	if _, err := RunSkillNew(dir, "review"); err == nil {
		t.Fatal("expected error creating duplicate skill")
	}
}
