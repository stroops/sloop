package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// peekPopupW/H are the default overlay size for peek — bigger than the HUD
// because you actually work inside the agent while it floats.
const (
	peekPopupW = "90%"
	peekPopupH = "80%"
)

// shellSingleQuote wraps s in single quotes safe for /bin/sh, escaping any
// embedded single quotes. Session names are simple, but never trust input.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildNestedAttachCmd returns a /bin/sh command that attaches to session as a
// fresh nested client. display-popup runs with $TMUX set, which would make a
// plain `tmux attach` refuse ("sessions should be nested with care"), so we
// clear TMUX/TMUX_PANE for just this command. Closing the popup kills this
// client → it detaches → the agent session keeps running.
func BuildNestedAttachCmd(session string) string {
	return fmt.Sprintf("env -u TMUX -u TMUX_PANE %s attach -t %s", Bin(), shellSingleQuote(session))
}

// NestedAttach attaches to session in the current TTY with TMUX cleared, used
// when we are already inside a popup (the keybind path) so no second popup opens.
func NestedAttach(session string) error {
	cmd := exec.Command(Bin(), "attach", "-t", session)
	cmd.Env = clearTmuxEnv(os.Environ())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// clearTmuxEnv drops TMUX and TMUX_PANE so a nested attach is permitted.
func clearTmuxEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "TMUX=") || strings.HasPrefix(e, "TMUX_PANE=") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Peek floats session's live pane over the current pane in a display-popup
// (must be run inside tmux ≥ 3.2). The overlay closes back to your work on exit.
func Peek(session string) error {
	cmd := exec.Command(Bin(), BuildPopupArgs(peekPopupW, peekPopupH, BuildNestedAttachCmd(session))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// BuildPeekBindArgs binds <prefix> <key> to open the peek popup running command.
func BuildPeekBindArgs(key, command string) []string {
	return append([]string{"bind-key", key}, BuildPopupArgs(peekPopupW, peekPopupH, command)...)
}

// BindPeek binds the peek key on the live tmux server.
func BindPeek(key, command string) error {
	return Run(BuildPeekBindArgs(key, command)...)
}
