package commands

import (
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/runner"
)

func TestResolveStartDirFromRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	// init registered the workspace under base(dir).
	got, err := resolveStartDir("/somewhere/else", filepath.Base(dir))
	if err != nil {
		t.Fatalf("resolveStartDir: %v", err)
	}
	wantAbs, _ := filepath.Abs(dir)
	gotAbs, _ := filepath.Abs(got)
	if gotAbs != wantAbs {
		t.Fatalf("want %s, got %s", wantAbs, gotAbs)
	}
}

func TestResolveStartDirNoFlagReturnsCwd(t *testing.T) {
	got, err := resolveStartDir("/here", "")
	if err != nil || got != "/here" {
		t.Fatalf("want /here, got %s err=%v", got, err)
	}
}

func TestSelectRunnerPrefersExecWhenNoTmux(t *testing.T) {
	// selectRunner returns a TmuxRunner only when tmux is available; otherwise ExecRunner.
	r := selectRunner("backend", "claude")
	if runner.TmuxAvailable() {
		if _, ok := r.(runner.TmuxRunner); !ok {
			t.Fatalf("expected TmuxRunner when tmux present")
		}
	} else {
		if _, ok := r.(runner.ExecRunner); !ok {
			t.Fatalf("expected ExecRunner when tmux absent")
		}
	}
}
