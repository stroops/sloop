//go:build e2e

// Package e2e drives the real sloop binary against a real tmux server to cover
// the interactive / multiplexer features that unit tests can't. It is gated
// behind the `e2e` build tag, so `go test ./...` skips it; run with `make e2e`
// (or `go test -tags e2e ./e2e/...`). Requires tmux on PATH.
//
// Robustness: the binary is built once for the whole package, sessions are
// namespaced by PID (never touching the developer's own), and assertions poll
// with a timeout instead of fixed sleeps so a slow CI runner doesn't flake.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// prefix namespaces every tmux session this run creates.
var prefix = fmt.Sprintf("e2e%d_", os.Getpid())

func requireTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

// buildSloop builds the binary once for the whole package (cached).
var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func buildSloop(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "sloop-e2e-*")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "sloop")
		if out, err := exec.Command("go", "build", "-o", binPath, "github.com/stroops/sloop/cmd/sloop").CombinedOutput(); err != nil {
			binErr = fmt.Errorf("build: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return binPath
}

func tmuxRun(args ...string) { _ = exec.Command("tmux", args...).Run() }

func tmuxNew(t *testing.T, name string) {
	t.Helper()
	tmuxRun("kill-session", "-t", name)
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", "/tmp").CombinedOutput(); err != nil {
		t.Fatalf("new-session %s: %v\n%s", name, err, out)
	}
}

func tmuxType(name, literal string) {
	tmuxRun("send-keys", "-t", name, "-l", literal)
	tmuxRun("send-keys", "-t", name, "Enter")
}

func capture(name string) string {
	out, _ := exec.Command("tmux", "capture-pane", "-p", "-t", name).Output()
	return string(out)
}

func hasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// runSloop runs the binary with an isolated HOME (separate registry/DB) and
// returns combined output + error, without failing the test (for polling).
func runSloop(bin, home string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home, "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// sloop is runSloop that fails the test on error (for setup/action steps).
func sloop(t *testing.T, bin, home string, args ...string) string {
	t.Helper()
	out, err := runSloop(bin, home, args...)
	if err != nil {
		t.Fatalf("sloop %v: %v\n%s", args, err, out)
	}
	return out
}

// waitFor polls fn until it returns true or the timeout elapses.
func waitFor(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestFleetAwarenessAndActions(t *testing.T) {
	requireTmux(t)
	bin := buildSloop(t)
	home := t.TempDir()

	wait := prefix + "wait__claude"
	tmuxNew(t, wait)
	tmuxType(wait, "printf 'Apply changes? (y/n) '; cat")
	t.Cleanup(func() { tmuxRun("kill-session", "-t", wait) })

	// ps reads the prompt + offers answer keys.
	waitFor(t, "ps to show the prompt and answer keys", func() bool {
		out, _ := runSloop(bin, home, "ps")
		return strings.Contains(out, prefix+"wait") && strings.Contains(out, "answer: [y]Yes [n]No")
	})

	// approve --waiting sends the affirmative ("y") into the pane.
	sloop(t, bin, home, "approve", "--waiting")
	waitFor(t, "approve to deliver 'y'", func() bool {
		return strings.Count(capture(wait), "y") >= 2 // "(y/n)" has one; the answer adds more
	})

	// kill removes it.
	sloop(t, bin, home, "kill", wait, "-y")
	waitFor(t, "session to be killed", func() bool { return !hasSession(wait) })
}

func TestAdoptExternalSession(t *testing.T) {
	requireTmux(t)
	bin := buildSloop(t)
	home := t.TempDir()

	ext := prefix + "external"
	tmuxNew(t, ext)
	adopted := prefix + "ws__claude"
	t.Cleanup(func() {
		tmuxRun("kill-session", "-t", ext)
		tmuxRun("kill-session", "-t", adopted)
	})

	// ps --all surfaces the unmanaged session.
	waitFor(t, "ps --all to list the external session", func() bool {
		out, _ := runSloop(bin, home, "ps", "--all")
		return strings.Contains(out, ext)
	})

	// adopt renames it into the convention.
	sloop(t, bin, home, "adopt", ext, "-w", prefix+"ws", "--as", "claude")
	waitFor(t, "adopt to rename the session", func() bool {
		return !hasSession(ext) && hasSession(adopted)
	})
}

func TestPopupSetupBindsKey(t *testing.T) {
	requireTmux(t)
	bin := buildSloop(t)
	home := t.TempDir()

	// a server must exist for list-keys to report binds.
	dummy := prefix + "popup_test"
	tmuxNew(t, dummy)
	t.Cleanup(func() { tmuxRun("kill-session", "-t", dummy) })

	key := "F12" // unlikely to clash with the developer's binds
	t.Cleanup(func() { tmuxRun("unbind-key", key) })

	out := sloop(t, bin, home, "popup", "setup", "--key", key)
	keys, _ := exec.Command("tmux", "list-keys").Output()
	if !strings.Contains(string(keys), "display-popup") || !strings.Contains(string(keys), key) {
		t.Fatalf("popup setup did not bind %s to display-popup\nsloop output:\n%s", key, out)
	}
}

func TestStatuslineSetup(t *testing.T) {
	requireTmux(t)
	bin := buildSloop(t)
	home := t.TempDir()

	sess := prefix + "sl__claude"
	tmuxNew(t, sess)
	t.Cleanup(func() { tmuxRun("kill-session", "-t", sess) })

	// the formatter renders a readable line for the session.
	if out := sloop(t, bin, home, "statusline", sess); !strings.Contains(out, "⚓ "+prefix+"sl claude") {
		t.Fatalf("statusline output: %q", out)
	}
	// setup points the session's status-right back at sloop.
	sloop(t, bin, home, "statusline", "setup", sess)
	got, _ := exec.Command("tmux", "show-options", "-t", sess, "status-right").Output()
	if !strings.Contains(string(got), "statusline "+sess) {
		t.Fatalf("status-right not set: %q", got)
	}
}

func TestFleetKeysAutoBound(t *testing.T) {
	requireTmux(t)
	bin := buildSloop(t)
	home := t.TempDir()

	// Start from a clean slate: a previous run may have left @sloop_peek_key set,
	// which would make EnsureFleetKeys a no-op.
	tmuxRun("set-option", "-gu", "@sloop_peek_key")
	tmuxRun("set-option", "-gu", "@sloop_hud_key")

	// adopt creates a managed session and runs the same SetStatusLine +
	// EnsureFleetKeys path as `run`, without needing a real AI CLI installed.
	ext := prefix + "keys_ext"
	tmuxNew(t, ext)
	adopted := prefix + "kws__claude"
	t.Cleanup(func() {
		tmuxRun("kill-session", "-t", ext)
		tmuxRun("kill-session", "-t", adopted)
		for _, k := range []string{"j", "a", "f", "p", "h", "g", "G"} {
			tmuxRun("unbind-key", k)
		}
		tmuxRun("set-option", "-gu", "@sloop_peek_key")
		tmuxRun("set-option", "-gu", "@sloop_hud_key")
	})

	sloop(t, bin, home, "adopt", ext, "-w", prefix+"kws", "--as", "claude")

	waitFor(t, "auto-bind to record @sloop_peek_key", func() bool {
		out, _ := exec.Command("tmux", "show-options", "-gv", "@sloop_peek_key").Output()
		return strings.TrimSpace(string(out)) != ""
	})
	keys, _ := exec.Command("tmux", "list-keys", "-T", "prefix").Output()
	if !strings.Contains(string(keys), "peek --in-popup") {
		t.Fatalf("peek not auto-bound to a prefix key:\n%s", keys)
	}
}
