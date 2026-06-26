package runner

import (
	"strconv"
	"strings"
	"time"
)

// TmuxSession is one row of `tmux list-sessions`.
type TmuxSession struct {
	Name     string
	Attached bool
	Windows  int
	Activity time.Time
}

// tmuxListFormat keeps `tmux list-sessions` output stable and tab-separated so
// ParseSessions can read it. The \t bytes are literal in the format string.
const tmuxListFormat = "#{session_name}\t#{session_attached}\t#{session_windows}\t#{session_activity}"

func BuildTmuxListArgs() []string {
	return []string{"list-sessions", "-F", tmuxListFormat}
}

// BuildTmuxSwitchArgs switches the current tmux client to a session — used when
// jumping between sessions while already inside tmux (attach can't nest).
func BuildTmuxSwitchArgs(session string) []string {
	return []string{"switch-client", "-t", session}
}

// ParseSessions parses tmuxListFormat output. Malformed lines are skipped.
func ParseSessions(raw string) []TmuxSession {
	var out []TmuxSession
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
		out = append(out, TmuxSession{
			Name:     f[0],
			Attached: strings.TrimSpace(f[1]) == "1",
			Windows:  win,
			Activity: time.Unix(secs, 0),
		})
	}
	return out
}
