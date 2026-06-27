package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func TmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TmuxSessionName(workspace, tool string) string {
	return sanitize(workspace) + "__" + sanitize(tool)
}

// DetachHint is the one place that explains how to detach (hide the agent and
// return to the terminal), using the user's actual tmux prefix. Shared by run,
// attach, and ps so the wording stays consistent.
func DetachHint() string {
	return fmt.Sprintf("\033[36m💡 detach (hide this agent, keep it running): press \033[1m%s\033[0m\033[36m then \033[1md\033[0m", TmuxPrefix())
}

// DetachLine is a compact one-liner for menu/list footers.
func DetachLine() string {
	return fmt.Sprintf("detach: %s then d", TmuxPrefix())
}

func TmuxPrefix() string {
	out, err := exec.Command("tmux", "show-options", "-g", "prefix").Output()
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

// BuildTmuxNewArgs builds `tmux new-session -A -s <session> -c <dir> <command> <args...>`.
// -A attaches if the session already exists, otherwise creates it.
func BuildTmuxNewArgs(session string, s Spec) []string {
	args := []string{"new-session", "-A", "-s", session, "-c", s.Dir, s.Command}
	return append(args, s.Args...)
}

func BuildTmuxAttachArgs(session string) []string {
	return []string{"attach", "-t", session}
}

type TmuxRunner struct {
	Session string
}

func (r TmuxRunner) Launch(s Spec) error {
	fmt.Printf("\n%s\n\n", DetachHint())

	cmd := exec.Command("tmux", BuildTmuxNewArgs(r.Session, s)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
