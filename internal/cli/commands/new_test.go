package commands

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/runner"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
)

// fakeDetachedRunner mimics tmux.DetachedRunner: Launch returns at creation.
type fakeDetachedRunner struct{ got runner.Spec }

func (f *fakeDetachedRunner) Launch(s runner.Spec) error { f.got = s; return nil }
func (f *fakeDetachedRunner) Detached() bool             { return true }

func lastSession(t *testing.T) session.Session {
	t.Helper()
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		t.Fatalf("GlobalDBPath: %v", err)
	}
	store, err := session.Open(dbPath)
	if err != nil {
		t.Fatalf("session.Open: %v", err)
	}
	defer func() { _ = store.Close() }()
	sessions, err := store.ListSessions(1)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListSessions: got %d sessions, err=%v", len(sessions), err)
	}
	return sessions[0]
}

// A detached launch returns as soon as the session is created; the agent is
// still running, so the history row must stay open (no EndedAt).
func TestResolveAndLaunchDetachedKeepsSessionOpen(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if _, err := RunInit(dir, []string{"claude"}, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	fr := &fakeDetachedRunner{}
	if err := resolveAndLaunch(dir, "claude", "", "", "", "", nil, nil, "", fr); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}
	if fr.got.Command != "claude" {
		t.Fatalf("want command claude, got %q", fr.got.Command)
	}
	if got := lastSession(t); got.EndedAt != nil {
		t.Fatalf("detached launch must leave the session open, got EndedAt=%v", got.EndedAt)
	}
}

// An attached launch returns when the session ends, so the row is closed.
func TestResolveAndLaunchAttachedClosesSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if _, err := RunInit(dir, []string{"claude"}, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	if err := resolveAndLaunch(dir, "claude", "", "", "", "", nil, nil, "", &fakeRunner{}); err != nil {
		t.Fatalf("resolveAndLaunch: %v", err)
	}
	if got := lastSession(t); got.EndedAt == nil {
		t.Fatalf("attached launch must close the session row")
	}
}

// DetachedRunner reports detached only after Launch actually creates a
// session, so a raced "already existed" no-op still closes its history row.
var _ runner.Detacher = (*tmux.DetachedRunner)(nil)

func TestLaunchIsDetached(t *testing.T) {
	if launchIsDetached(&fakeRunner{}) {
		t.Fatalf("plain runner must not be detached")
	}
	if !launchIsDetached(&fakeDetachedRunner{}) {
		t.Fatalf("fake detached runner must be detached")
	}
	if launchIsDetached(&tmux.DetachedRunner{Session: "x"}) {
		t.Fatalf("DetachedRunner that launched nothing must not report a running session")
	}
	if launchIsDetached(tmux.Runner{Session: "x"}) {
		t.Fatalf("tmux.Runner must not be detached")
	}
	if launchIsDetached(runner.ExecRunner{}) {
		t.Fatalf("ExecRunner must not be detached")
	}
}

// `new` shares run's grammar and knobs, differing only in the attach default.
func TestNewCmdFlagsMirrorRun(t *testing.T) {
	// Flag registration happens in Register*(root); do it once for both here.
	if newCmd.Flags().Lookup("attach") == nil {
		RegisterNew(&cobra.Command{})
	}
	if runCmd.Flags().Lookup("split") == nil {
		RegisterRun(&cobra.Command{})
	}
	for _, name := range []string{"workspace", "provider", "model", "effort", "task", "name", "env", "new"} {
		if newCmd.Flags().Lookup(name) == nil {
			t.Fatalf("new is missing run's --%s flag", name)
		}
		if runCmd.Flags().Lookup(name) == nil {
			t.Fatalf("run lost its --%s flag", name)
		}
	}
	if newCmd.Flags().Lookup("attach") == nil {
		t.Fatalf("new must have --attach to flip into run behavior")
	}
	if newCmd.Flags().Lookup("split") != nil {
		t.Fatalf("--split is a run-only flag")
	}
}

func TestNewCmdRejectsTwoTargets(t *testing.T) {
	if err := newCmd.Args(newCmd, []string{"claude", "codex"}); err == nil {
		t.Fatalf("want error for two positional targets")
	}
	if err := newCmd.Args(newCmd, []string{"claude"}); err != nil {
		t.Fatalf("one target must be accepted: %v", err)
	}
}
