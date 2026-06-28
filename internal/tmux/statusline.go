package tmux

import (
	"fmt"
	"os"
	"strings"
)

// statusLineEnabled reports whether sloop should style a session's status bar.
// Power users with their own tmux status bar can keep it untouched by setting
// SLOOP_STATUSLINE=0 (or off/false/no) — sloop then never overrides status-left/
// status-right, even per session.
func statusLineEnabled() bool {
	switch strings.ToLower(os.Getenv("SLOOP_STATUSLINE")) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

// StatusRightFormat is the tmux status-right value that calls back into sloop to
// render this session's live status (re-run every status-interval).
func StatusRightFormat(exe, session string) string {
	return fmt.Sprintf("#(%s statusline %s)", exe, session)
}

// SetStatusLine points a single session's status bar at `sloop statusline`,
// refreshed every 2s. It is per-session (`set-option -t`), so it never touches
// the user's global tmux config (~/.tmux.conf) or other sessions, and only
// overrides status-left/status-right (not colors/theme). Best-effort, and a
// no-op when SLOOP_STATUSLINE is disabled so a custom status bar stays intact.
func SetStatusLine(session string) {
	if !statusLineEnabled() {
		return
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "sloop"
	}
	set := func(opt, val string) {
		_ = Run("set-option", "-t", session, opt, val)
	}
	set("status-right", StatusRightFormat(exe, session))
	set("status-right-length", "60")
	set("status-interval", "2")
	// status-left becomes a persistent "how to get back" tip, using the user's
	// real prefix, so anyone inside the session can see how to detach (return to
	// the fleet without stopping the agent).
	set("status-left", fmt.Sprintf(" ⚓ detach: %s d ", PrefixRaw()))
	set("status-left-length", "24")
}
