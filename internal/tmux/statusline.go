package tmux

import (
	"fmt"
	"os"
	"strings"
)

// statusLineEnabled reports whether sloop should style a session's status bar.
// Power users with their own tmux status bar can keep it untouched by setting
// SLOOP_STATUSLINE=0 (or off/false/no); sloop then never overrides status-left/
// status-right, even per session.
func statusLineEnabled() bool {
	switch strings.ToLower(os.Getenv("SLOOP_STATUSLINE")) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

// StatusSideFormat is the tmux status-left/right value that calls back into
// sloop to render one side of this session's live status bar (re-run every
// status-interval). side is "left" or "right".
func StatusSideFormat(exe, side, session string) string {
	return fmt.Sprintf("#(%s statusline %s %s)", exe, side, session)
}

// SetStatusLine points a single session's status bar at `sloop statusline`,
// refreshed every 2s. It is per-session (`set-option -t`), so it never touches
// the user's global tmux config (~/.tmux.conf) or other sessions. The left
// side carries everything worth knowing at a glance (identity, agent status,
// model, context %, git branch); the right side carries just a rotating hint.
// tmux's own window list, which would otherwise sit between them, is hidden:
// a sloop session is one window, so it only ever duplicated the identity the
// left side already shows. Best-effort, and a no-op when SLOOP_STATUSLINE is
// disabled so a custom status bar stays intact.
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
	// A quiet dark bar instead of tmux's default green: colour234 (near-black)
	// with dim grey text, so the colored status glyphs are what draws the eye.
	set("status-style", "bg=colour234,fg=colour245")
	set("status-interval", "2")
	set("status-left", StatusSideFormat(exe, "left", session))
	set("status-left-length", "120")
	set("status-right", StatusSideFormat(exe, "right", session))
	set("status-right-length", "40")
	// Empty window-status-format/separator collapses the middle window list
	// to nothing; a manually-opened extra window (Ctrl+b c) still exists and
	// is reachable, it just isn't listed here.
	set("window-status-format", "")
	set("window-status-current-format", "")
	set("window-status-separator", "")
}
