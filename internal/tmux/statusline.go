package tmux

import (
	"fmt"
	"os"
)

// StatusRightFormat is the tmux status-right value that calls back into sloop to
// render this session's live status (re-run every status-interval).
func StatusRightFormat(exe, session string) string {
	return fmt.Sprintf("#(%s statusline %s)", exe, session)
}

// SetStatusLine points a single session's status bar at `sloop statusline`,
// refreshed every 2s. It is per-session (`set-option -t`), so it never touches
// the user's global tmux config or other sessions. Best-effort.
func SetStatusLine(session string) {
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
