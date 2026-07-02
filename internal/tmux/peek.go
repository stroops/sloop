package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// peekPopupW/H are the default overlay size for peek, bigger than the HUD
// because you actually work inside the agent while it floats.
const (
	peekPopupW = "90%"
	peekPopupH = "80%"
)

// ShellQuote wraps s in single quotes safe for /bin/sh, escaping any
// embedded single quotes. Session names are simple, but never trust input.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildNestedAttachCmd returns a /bin/sh command that attaches to session as a
// fresh nested client. display-popup runs with $TMUX set, which would make a
// plain `tmux attach` refuse ("sessions should be nested with care"), so we
// clear TMUX/TMUX_PANE for just this command. Closing the popup kills this
// client → it detaches → the agent session keeps running.
func BuildNestedAttachCmd(session string) string {
	return fmt.Sprintf("env -u TMUX -u TMUX_PANE %s attach -t %s", Bin(), ShellQuote(Exact(session)))
}

// NestedAttach attaches to session in the current TTY with TMUX cleared, used
// when we are already inside a popup (the keybind path) so no second popup opens.
func NestedAttach(session string) error {
	cmd := exec.Command(Bin(), "attach", "-t", Exact(session))
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

// peekTitle is the caption shown on a peek popup's border (tmux ≥ 3.3): it marks
// the overlay as a peek and shows how to close it. session may be "" (the keybind
// path doesn't know the target until runtime) → a generic caption.
func peekTitle(session, prefix string) string {
	if session == "" {
		return fmt.Sprintf(" 👀 peek — %s d to close ", prefix)
	}
	return fmt.Sprintf(" 👀 peek · %s — %s d to close ", session, prefix)
}

// withTitle inserts `-T <title>` before the trailing `-E <command>` of a
// display-popup arg list, so the popup shows a titled border. A no-op when title
// is "". It assumes the last two args are `-E` and the command (true for
// BuildPopupArgs and BuildPeekBindArgs).
func withTitle(args []string, title string) []string {
	if title == "" || len(args) < 2 {
		return args
	}
	n := len(args)
	out := make([]string, 0, n+2)
	out = append(out, args[:n-2]...)
	out = append(out, "-T", title)
	out = append(out, args[n-2:]...)
	return out
}

// peekTitleNow resolves the live peek caption: empty when tmux can't render a
// popup title (< 3.3), else peekTitle with the user's real prefix.
func peekTitleNow(session string) string {
	if !TitleSupported() {
		return ""
	}
	return peekTitle(session, Prefix())
}

// Peek floats session's live pane over the current pane in a display-popup
// (must be run inside tmux ≥ 3.2). The overlay closes back to your work on exit.
func Peek(session string) error {
	args := withTitle(BuildPopupArgs(peekPopupW, peekPopupH, BuildNestedAttachCmd(session)), peekTitleNow(session))
	cmd := exec.Command(Bin(), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// BuildPeekBindArgs binds <prefix> <key> to open the peek popup running command.
func BuildPeekBindArgs(key, command string) []string {
	return append([]string{"bind-key", key}, BuildPopupArgs(peekPopupW, peekPopupH, command)...)
}

// BindPeek binds the peek key on the live tmux server, with a titled popup
// border when supported (the keybind path doesn't know the target session yet,
// so the caption is generic).
func BindPeek(key, command string) error {
	return Run(withTitle(BuildPeekBindArgs(key, command), peekTitleNow(""))...)
}
