package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/runner"
)

type fakeRunner struct{ got runner.Spec }

func (f *fakeRunner) Launch(s runner.Spec) error { f.got = s; return nil }

func TestResolveAndLaunchSyncsAndLaunches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	// Two tools → canonical workspace, so the CLAUDE.md pointer is delivered.
	if _, err := RunInit(dir, []string{"claude", "cursor"}, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	fr := &fakeRunner{}
	if err := resolveAndLaunch(dir, "claude", "", "", "", "", nil, nil, "", fr); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
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

func TestResolveAndLaunchPassesThroughArgs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if _, err := RunInit(dir, nil, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	fr := &fakeRunner{}
	if err := resolveAndLaunch(dir, "claude", "", "", "", "", []string{"--foo", "bar"}, nil, "", fr); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}
	if len(fr.got.Args) != 2 || fr.got.Args[0] != "--foo" || fr.got.Args[1] != "bar" {
		t.Fatalf("want passthrough args [--foo bar], got %v", fr.got.Args)
	}
}

func TestResolveAndLaunchInjectsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if _, err := RunInit(dir, nil, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	fr := &fakeRunner{}
	env := map[string]string{"CLAUDE_CONFIG_DIR": "/x"}
	if err := resolveAndLaunch(dir, "claude", "", "", "", "", nil, env, "sec", fr); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}
	if fr.got.Env["CLAUDE_CONFIG_DIR"] != "/x" {
		t.Fatalf("env not passed to Spec: %v", fr.got.Env)
	}
}
