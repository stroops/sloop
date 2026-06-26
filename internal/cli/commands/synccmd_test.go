package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/config"
)

func TestRunSyncWritesClaudeMd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	written, err := RunSync(dir, "claude")
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(b), "AGENTS.md") {
		t.Fatalf("CLAUDE.md should point to AGENTS.md:\n%s", string(b))
	}
	_ = written
}

func TestRunSyncAllDeliversEnabledTools(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	// Enable claude + cursor.
	if err := config.SaveProject(filepath.Join(dir, ".sloop"), &config.Project{
		Tools: []string{"claude", "cursor"}, DefaultTool: "claude",
	}); err != nil {
		t.Fatal(err)
	}
	lines, err := RunSyncAll(dir)
	if err != nil {
		t.Fatalf("RunSyncAll: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatalf("claude pointer missing: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "claude:") {
		t.Fatalf("expected per-tool prefixed output, got:\n%s", joined)
	}
}
