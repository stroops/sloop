//go:build e2e

// Package e2e drives the real sloop binary against a real tmux server to cover
// the interactive / multiplexer features that unit tests can't. It is gated
// behind the `e2e` build tag, so `go test ./...` skips it; run with `make e2e`
// (or `go test -tags e2e ./e2e/...`). Requires tmux on PATH.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// prefix namespaces every tmux session this run creates, so we never touch the
// developer's own sessions and parallel runs don't collide.
var prefix = fmt.Sprintf("e2e%d_", os.Getpid())

func requireTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

// buildSloop builds the binary once into a temp dir.
func buildSloop(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/sloop"
	out, err := exec.Command("go", "build", "-o", bin, "github.com/stroops/sloop/cmd/sloop").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func tmuxRun(t *testing.T, args ...string) {
	t.Helper()
	_ = exec.Command("tmux", args...).Run()
}

func tmuxNew(t *testing.T, name string) {
	t.Helper()
	tmuxRun(t, "kill-session", "-t", name)
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", "/tmp").CombinedOutput(); err != nil {
		t.Fatalf("new-session %s: %v\n%s", name, err, out)
	}
}

func tmuxType(t *testing.T, name, literal string) {
	t.Helper()
	tmuxRun(t, "send-keys", "-t", name, "-l", literal)
	tmuxRun(t, "send-keys", "-t", name, "Enter")
}

func capture(t *testing.T, name string) string {
	t.Helper()
	out, _ := exec.Command("tmux", "capture-pane", "-p", "-t", name).Output()
	return string(out)
}

func hasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// sloop runs the binary with an isolated HOME (separate registry/DB) and returns
// combined output. tmux sessions are global, hence the per-run prefix.
func sloop(t *testing.T, bin, home string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home, "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sloop %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestFleetAwarenessAndActions(t *testing.T) {
	requireTmux(t)
	bin := buildSloop(t)
	home := t.TempDir()

	wait := prefix + "wait__claude"
	tmuxNew(t, wait)
	tmuxType(t, wait, "printf 'Apply changes? (y/n) '; cat")
	t.Cleanup(func() { tmuxRun(t, "kill-session", "-t", wait) })
	time.Sleep(400 * time.Millisecond)

	// ps reads the prompt + offers answer keys.
	out := sloop(t, bin, home, "ps")
	if !strings.Contains(out, prefix+"wait") || !strings.Contains(out, "answer: [y]Yes [n]No") {
		t.Fatalf("ps did not show prompt/answers:\n%s", out)
	}

	// approve --waiting sends the affirmative.
	sloop(t, bin, home, "approve", "--waiting")
	time.Sleep(300 * time.Millisecond)
	if c := capture(t, wait); strings.Count(c, "\ny\n")+strings.Count(c, "y\n") == 0 {
		t.Fatalf("approve did not deliver 'y':\n%s", c)
	}

	// kill removes it.
	sloop(t, bin, home, "kill", wait, "-y")
	if hasSession(wait) {
		t.Fatalf("kill left the session alive")
	}
}

func TestAdoptExternalSession(t *testing.T) {
	requireTmux(t)
	bin := buildSloop(t)
	home := t.TempDir()

	ext := prefix + "external"
	tmuxNew(t, ext)
	adopted := prefix + "ws__claude"
	t.Cleanup(func() {
		tmuxRun(t, "kill-session", "-t", ext)
		tmuxRun(t, "kill-session", "-t", adopted)
	})

	// ps --all surfaces the unmanaged session.
	if out := sloop(t, bin, home, "ps", "--all"); !strings.Contains(out, ext) {
		t.Fatalf("ps --all did not list external session:\n%s", out)
	}

	// adopt renames it into the convention.
	sloop(t, bin, home, "adopt", ext, "-w", prefix+"ws", "--as", "claude")
	if hasSession(ext) || !hasSession(adopted) {
		t.Fatalf("adopt did not rename %s → %s", ext, adopted)
	}
}

func TestPopupSetupBindsKey(t *testing.T) {
	requireTmux(t)
	bin := buildSloop(t)
	home := t.TempDir()
	
	dummy := prefix + "popup_test"
	tmuxNew(t, dummy)
	t.Cleanup(func() { tmuxRun(t, "kill-session", "-t", dummy) })

	key := "F12" // unlikely to clash with the developer's binds
	t.Cleanup(func() { tmuxRun(t, "unbind-key", key) })

	outSloop := sloop(t, bin, home, "popup", "setup", "--key", key)
	out, err := exec.Command("tmux", "list-keys").Output()
	if !strings.Contains(string(out), "display-popup") || !strings.Contains(string(out), key) {
		t.Fatalf("popup setup did not bind %s to display-popup\nerr: %v\nsloop output:\n%s", key, err, outSloop)
	}
}
