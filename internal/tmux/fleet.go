package tmux

import (
	"strconv"
	"strings"
	"time"
)

// Session is one row of `tmux list-sessions`.
type Session struct {
	Name     string
	Attached bool
	Windows  int
	Activity time.Time
}

// listFormat keeps `tmux list-sessions` output stable and tab-separated so
// ParseSessions can read it. The \t bytes are literal in the format string.
const listFormat = "#{session_name}\t#{session_attached}\t#{session_windows}\t#{session_activity}"

func BuildListArgs() []string {
	return []string{"list-sessions", "-F", listFormat}
}

// BuildSwitchArgs switches the current tmux client to a session, used when
// jumping between sessions while already inside tmux (attach can't nest).
func BuildSwitchArgs(session string) []string {
	return []string{"switch-client", "-t", session}
}

// BuildCaptureArgs reads the visible content of a session's active pane.
// This only ever reads your own terminal output, never the provider's API or
// internals, so it stays within what any AI tool's terms allow.
func BuildCaptureArgs(session string) []string {
	return []string{"capture-pane", "-p", "-t", session}
}

// LastNonEmptyLine returns the last non-blank, trimmed line of captured pane
// text (the agent's most recent output), or "" if there is none.
func LastNonEmptyLine(raw string) string {
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

// ParseSessions parses listFormat output. Malformed lines are skipped.
func ParseSessions(raw string) []Session {
	var out []Session
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		win, _ := strconv.Atoi(strings.TrimSpace(f[2]))
		secs, _ := strconv.ParseInt(strings.TrimSpace(f[3]), 10, 64)
		out = append(out, Session{
			Name:     f[0],
			Attached: strings.TrimSpace(f[1]) == "1",
			Windows:  win,
			Activity: time.Unix(secs, 0),
		})
	}
	return out
}
