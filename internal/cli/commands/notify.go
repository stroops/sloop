package commands

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// osNotify shows a best-effort desktop notification about one of your own
// sessions (used by `ps --watch`). It shells out to the OS notifier (osascript
// on macOS, notify-send on Linux, a PowerShell balloon on Windows) and silently
// does nothing if that's unavailable; a convenience, never a hard dependency,
// never touches the provider.
func osNotify(title, message string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("osascript", "-e",
			"display notification "+quote(message)+" with title "+quote(title))
	case "linux":
		cmd = exec.Command("notify-send", title, message)
	case "windows":
		// Built-in balloon tip via WinForms; no extra modules required.
		ps := fmt.Sprintf(
			"Add-Type -AssemblyName System.Windows.Forms;"+
				"Add-Type -AssemblyName System.Drawing;"+
				"$n=New-Object System.Windows.Forms.NotifyIcon;"+
				"$n.Icon=[System.Drawing.SystemIcons]::Information;$n.Visible=$true;"+
				"$n.ShowBalloonTip(5000,'%s','%s','Info')",
			psEscape(title), psEscape(message))
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	default:
		return
	}
	_ = cmd.Run()
}

// psEscape makes a string safe inside a single-quoted PowerShell literal.
func psEscape(s string) string { return strings.ReplaceAll(s, "'", "''") }

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
