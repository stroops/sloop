package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/runner"
)

type fakeRunner struct{ got runner.Spec }

func (f *fakeRunner) Launch(s runner.Spec) error { f.got = s; return nil }

func TestRunRunSyncsAndLaunches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if _, err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	fr := &fakeRunner{}
	if err := RunRun(dir, "claude", nil, fr); err != nil {
		t.Fatalf("RunRun: %v", err)
	}

	// Launched claude at the workspace root.
	if fr.got.Command != "claude" {
		t.Fatalf("want command claude, got %q", fr.got.Command)
	}
	wantDir, _ := filepath.Abs(dir)
	gotDir, _ := filepath.Abs(fr.got.Dir)
	if gotDir != wantDir {
		t.Fatalf("want dir %s, got %s", wantDir, gotDir)
	}
	// Sync ran as part of run.
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md after run: %v", err)
	}
}

func TestRunRunPassesThroughArgs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if _, err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	fr := &fakeRunner{}
	if err := RunRun(dir, "claude", []string{"--model", "opus"}, fr); err != nil {
		t.Fatalf("RunRun: %v", err)
	}
	if len(fr.got.Args) != 2 || fr.got.Args[0] != "--model" || fr.got.Args[1] != "opus" {
		t.Fatalf("want passthrough args [--model opus], got %v", fr.got.Args)
	}
}
