package tmux

import (
	"fmt"
	"os"
	"os/exec"
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
		_ = exec.Command(Bin(), "set-option", "-t", session, opt, val).Run()
	}
	set("status-right", StatusRightFormat(exe, session))
	set("status-right-length", "60")
	set("status-interval", "2")
}
