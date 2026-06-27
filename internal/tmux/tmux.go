package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/stroops/sloop/internal/runner"
)

// muxCandidates are the tmux-compatible multiplexers sloop drives, in
// preference order. psmux (native Windows, Rust) speaks the same CLI as tmux,
// so the whole backend works on Windows by just picking a different binary.
var muxCandidates = []string{"tmux", "psmux"}

var (
	binOnce sync.Once
	binName string
)

// Bin returns the multiplexer binary to invoke: $SLOOP_MUX if set, else the
// first of tmux/psmux found on PATH, else "tmux" (so callers still get a
// sensible command; Available() reports whether it actually exists).
func Bin() string {
	binOnce.Do(func() {
		binName = resolveBin(os.Getenv("SLOOP_MUX"), func(c string) bool {
			_, err := exec.LookPath(c)
			return err == nil
		})
	})
	return binName
}

// resolveBin is the pure selection logic behind Bin (testable without PATH).
func resolveBin(env string, onPath func(string) bool) string {
	if env != "" {
		return env
	}
	for _, c := range muxCandidates {
		if onPath(c) {
			return c
		}
	}
	return "tmux"
}

// Available reports whether a usable multiplexer (tmux or psmux) is installed.
func Available() bool {
	_, err := exec.LookPath(Bin())
	return err == nil
}

func SessionName(workspace, tool string) string {
	return sanitize(workspace) + "__" + sanitize(tool)
}

// DetachHint is the one place that explains how to detach (hide the agent and
// return to the terminal), using the user's actual tmux prefix. Shared by run,
// attach, and ps so the wording stays consistent.
func DetachHint() string {
	return fmt.Sprintf("\033[36m💡 detach (hide this agent, keep it running): press \033[1m%s\033[0m\033[36m then \033[1md\033[0m", Prefix())
}

// DetachLine is a compact one-liner for menu/list footers.
func DetachLine() string {
	return fmt.Sprintf("detach: %s then d", Prefix())
}

func Prefix() string {
	out, err := exec.Command(Bin(), "show-options", "-g", "prefix").Output()
	if err != nil {
		return "Ctrl+b"
	}
	s := strings.TrimSpace(string(out))
	parts := strings.Split(s, " ")
	if len(parts) == 2 {
		p := parts[1]
		if strings.HasPrefix(p, "C-") {
			return "Ctrl+" + p[2:]
		}
		return p
	}
	return "Ctrl+b"
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// BuildNewArgs builds `tmux new-session -A -s <session> -c <dir> <command> <args...>`.
// -A attaches if the session already exists, otherwise creates it.
func BuildNewArgs(session string, s runner.Spec) []string {
	args := []string{"new-session", "-A", "-s", session, "-c", s.Dir, s.Command}
	return append(args, s.Args...)
}

func BuildAttachArgs(session string) []string {
	return []string{"attach", "-t", session}
}

func BuildKillArgs(session string) []string {
	return []string{"kill-session", "-t", session}
}

// Kill ends a session (the agent stops with it).
func Kill(session string) error {
	return exec.Command(Bin(), BuildKillArgs(session)...).Run()
}

func BuildRenameArgs(old, name string) []string {
	return []string{"rename-session", "-t", old, name}
}

// Rename renames a running session (used to adopt an external session into the
// sloop `<workspace>__<tool>` convention).
func Rename(old, name string) error {
	return exec.Command(Bin(), BuildRenameArgs(old, name)...).Run()
}

// SessionPath reports a session's active pane working directory (best-effort),
// so an adopted session can be registered against its repo path.
func SessionPath(session string) string {
	out, err := exec.Command(Bin(), "display-message", "-t", session, "-p", "#{pane_current_path}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type Runner struct {
	Session string
}

func (r Runner) Launch(s runner.Spec) error {
	fmt.Printf("\n%s\n\n", DetachHint())

	// Create the session detached so we can give it sloop's own status bar
	// before attaching; then attach (or switch if already inside tmux).
	if !hasSession(r.Session) {
		create := append([]string{"new-session", "-d", "-s", r.Session, "-c", s.Dir, s.Command}, s.Args...)
		if err := exec.Command(Bin(), create...).Run(); err != nil {
			return err
		}
		SetStatusLine(r.Session)
	}
	args := BuildAttachArgs(r.Session)
	if os.Getenv("TMUX") != "" {
		args = BuildSwitchArgs(r.Session)
	}
	cmd := exec.Command(Bin(), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
