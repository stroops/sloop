package sync

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/profile"
)

func TestStaleWhenOutputMissing(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	mustWrite(t, filepath.Join(sloopDir, "context", "a.md"), "x")
	m := adapter.Manifest{Outputs: []adapter.Output{{Path: "CLAUDE.md", Template: "default"}}}
	stale, err := Stale(root, sloopDir, m, profile.Profile{Context: "all"})
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	if !stale {
		t.Fatal("missing output should be stale")
	}
}

func TestFreshAfterWrite(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	mustWrite(t, filepath.Join(sloopDir, "context", "a.md"), "x")
	// Generate the output after the source.
	time.Sleep(10 * time.Millisecond)
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "rendered")
	m := adapter.Manifest{Outputs: []adapter.Output{{Path: "CLAUDE.md", Template: "default"}}}
	stale, err := Stale(root, sloopDir, m, profile.Profile{Context: "all"})
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	if stale {
		t.Fatal("output newer than source should be fresh")
	}
}
