package runner

import (
	"os/exec"
	"runtime"
)

// Notify shows a best-effort desktop notification about one of your own
// sessions. It shells out to the OS notifier (osascript on macOS, notify-send
// on Linux) and silently does nothing if that tool is missing — it is a
// convenience, never a hard dependency, and never touches the AI provider.
func Notify(title, message string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("osascript", "-e",
			"display notification "+quote(message)+" with title "+quote(title))
	case "linux":
		cmd = exec.Command("notify-send", title, message)
	default:
		return
	}
	_ = cmd.Run()
}

// quote wraps s in double quotes for AppleScript, escaping embedded quotes and
// backslashes so a session name can't break out of the string.
func quote(s string) string {
	out := make([]rune, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	out = append(out, '"')
	return string(out)
}
