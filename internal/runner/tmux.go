package runner

import (
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
	cmd := exec.Command("tmux", BuildTmuxNewArgs(r.Session, s)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
