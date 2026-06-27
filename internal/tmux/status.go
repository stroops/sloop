package tmux

import "strings"

// AgentStatus is a best-effort classification of what an AI session is doing,
// derived only from the visible content of your own pane (capture-pane). It is a
// non-invasive local signal: sloop never reads the provider's API or internals.
type AgentStatus int

const (
	StatusUnknown AgentStatus = iota // no pane text / can't tell
	StatusIdle                       // at an empty prompt, nothing pending
	StatusWorking                    // actively producing output / running a tool
	StatusWaiting                    // blocked on you (approval, a question) — "needs me"
)

func (s AgentStatus) String() string {
	switch s {
	case StatusIdle:
		return "idle"
	case StatusWorking:
		return "working"
	case StatusWaiting:
		return "waiting"
	default:
		return "unknown"
	}
}

// NeedsAttention reports whether the agent is blocked waiting for the user.
func (s AgentStatus) NeedsAttention() bool { return s == StatusWaiting }

// statusScanLines is how many trailing non-empty lines we consider; a prompt or
// spinner is almost always within the last handful of lines.
const statusScanLines = 8

// waitingMarkers signal the agent is blocked on the user. Matched case-insensitively.
var waitingMarkers = []string{
	"do you want to",
	"(y/n)", "[y/n]", "y/n)", "(yes/no)",
	"press enter",
	"approve", "approval",
	"continue?",
	"waiting for your",
	"allow this",
	"proceed?",
	"1. yes", "❯ 1.", "› 1.",
	"would you like",
	"confirm",
}

// workingMarkers signal the agent is actively running. Matched case-insensitively.
var workingMarkers = []string{
	"esc to interrupt",
	"ctrl+c to", "ctrl-c to",
	"thinking", "working…", "working...",
	"generating", "running",
	"tokens", "esc to cancel",
}

// spinnerRunes are common braille spinner glyphs AI CLIs animate while busy.
const spinnerRunes = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏⣾⣽⣻⢿⡿⣟⣯⣷◐◓◑◒"

// ClassifyStatus inspects the tail of a captured pane and returns a best-effort
// status. Waiting (needs you) takes precedence over working, which takes
// precedence over idle. Empty input is StatusUnknown.
func ClassifyStatus(pane string) AgentStatus {
	tail := lastLines(pane, statusScanLines)
	if tail == "" {
		return StatusUnknown
	}
	low := strings.ToLower(tail)
	for _, m := range waitingMarkers {
		if strings.Contains(low, m) {
			return StatusWaiting
		}
	}
	for _, m := range workingMarkers {
		if strings.Contains(low, m) {
			return StatusWorking
		}
	}
	if strings.ContainsAny(tail, spinnerRunes) {
		return StatusWorking
	}
	return StatusIdle
}

// lastLines returns the last n non-empty (trimmed) lines of s, joined by "\n".
func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	var kept []string
	for i := len(lines) - 1; i >= 0 && len(kept) < n; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			kept = append(kept, t)
		}
	}
	// kept is reversed, but order doesn't matter for substring matching.
	return strings.Join(kept, "\n")
}
