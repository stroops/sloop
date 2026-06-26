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
	// Add a distinctive line to context so we can detect it in output.
	ctx := filepath.Join(dir, ".sloop", "context", "project.md")
	if err := os.WriteFile(ctx, []byte("MARKER-CONTEXT"), 0o644); err != nil {
		t.Fatal(err)
	}

	written, err := RunSync(dir, "claude")
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 written file, got %v", written)
	}
	b, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(b), "MARKER-CONTEXT") {
		t.Fatalf("CLAUDE.md missing context:\n%s", string(b))
	}
}
