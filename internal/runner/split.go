package runner

import (
	"os"
	"os/exec"
)

// Split-pane tmux arg builders (pure; the exec sequence lives in LaunchSplit).

func BuildTmuxSplitNew(session, dir, cmd string) []string {
	return []string{"new-session", "-d", "-s", session, "-c", dir, cmd}
}

func BuildTmuxSplitAdd(session, dir, cmd string) []string {
	return []string{"split-window", "-t", session, "-c", dir, cmd}
}

func BuildTmuxTiledLayout(session string) []string {
	return []string{"select-layout", "-t", session, "tiled"}
}

func tmuxHasSession(session string) bool {
	return exec.Command("tmux", "has-session", "-t", session).Run() == nil
}

// LaunchSplit creates (idempotently) a tmux session with one pane per command,
// all rooted at dir and tiled, then attaches — or switches the client when
// already inside tmux, since attach cannot nest.
func LaunchSplit(session, dir string, cmds []string) error {
	if len(cmds) == 0 {
		return nil
	}
	if !tmuxHasSession(session) {
		if err := exec.Command("tmux", BuildTmuxSplitNew(session, dir, cmds[0])...).Run(); err != nil {
			return err
		}
		for _, c := range cmds[1:] {
			if err := exec.Command("tmux", BuildTmuxSplitAdd(session, dir, c)...).Run(); err != nil {
				return err
			}
			// Re-tile after each pane so tmux always has room for the next.
			_ = exec.Command("tmux", BuildTmuxTiledLayout(session)...).Run()
		}
	}
	args := BuildTmuxAttachArgs(session)
	if os.Getenv("TMUX") != "" {
		args = BuildTmuxSwitchArgs(session)
	}
	c := exec.Command("tmux", args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}
