package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
