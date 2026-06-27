package tmux

import (
	"regexp"
	"strings"

	"github.com/stroops/sloop/internal/adapter"
)

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
func ClassifyStatus(pane string, manifest adapter.Manifest) AgentStatus {
	tail := lastLines(pane, statusScanLines)
	if tail == "" {
		return StatusUnknown
	}
	low := strings.ToLower(tail)
	for _, m := range append(waitingMarkers, manifest.Heuristics.Waiting...) {
		if strings.Contains(low, strings.ToLower(m)) {
			return StatusWaiting
		}
	}
	for _, m := range append(workingMarkers, manifest.Heuristics.Working...) {
		if strings.Contains(low, strings.ToLower(m)) {
			return StatusWorking
		}
	}
	if strings.ContainsAny(tail, spinnerRunes) {
		return StatusWorking
	}
	return StatusIdle
}

// Answer is a choice the agent is offering at a blocking prompt. Key is what to
// send to pick it ("y", "n", "1"…); Key "" means "just press Enter".
type Answer struct {
	Key   string
	Label string
}

// numberedChoice matches a menu line like "❯ 1. Yes" or "  2) No".
var numberedChoice = regexp.MustCompile(`^[❯›>*•\-\s]*([0-9])[.)]\s+(\S.*)$`)

// ynPatterns are lowercased yes/no prompt markers (covers [y/N], (Y/n), etc.).
var ynPatterns = []string{"(y/n)", "[y/n]", "y/n)", "(yes/no)", "[yes/no]"}

// ParseAnswers extracts the choices an agent is offering at a blocking prompt,
// heuristically, from the tail of its own pane. Empty when nothing recognized.
func ParseAnswers(pane string) []Answer {
	lines := tailLines(pane, 12)

	var numbered []Answer
	seen := map[string]bool{}
	for _, ln := range lines {
		if m := numberedChoice.FindStringSubmatch(strings.TrimSpace(ln)); m != nil && !seen[m[1]] {
			seen[m[1]] = true
			numbered = append(numbered, Answer{Key: m[1], Label: cleanLabel(m[2])})
		}
	}
	if len(numbered) >= 2 {
		return numbered
	}

	low := strings.ToLower(strings.Join(lines, "\n"))
	for _, p := range ynPatterns {
		if strings.Contains(low, p) {
			return []Answer{{Key: "y", Label: "Yes"}, {Key: "n", Label: "No"}}
		}
	}
	if strings.Contains(low, "press enter") || strings.Contains(low, "to continue") {
		return []Answer{{Key: "", Label: "continue"}}
	}
	return nil
}

// PromptLine returns the question the agent is blocked on (best-effort).
func PromptLine(pane string) string {
	lines := tailLines(pane, 12)
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); strings.HasSuffix(t, "?") {
			return cleanLabel(t)
		}
	}
	for i, ln := range lines {
		if numberedChoice.MatchString(strings.TrimSpace(ln)) {
			if i > 0 {
				return cleanLabel(strings.TrimSpace(lines[i-1]))
			}
			break
		}
	}
	if len(lines) > 0 {
		return cleanLabel(strings.TrimSpace(lines[len(lines)-1]))
	}
	return ""
}

// AffirmativeAnswer returns the "approve/yes" choice among answers, if any.
func AffirmativeAnswer(answers []Answer) (Answer, bool) {
	for _, a := range answers {
		l := strings.ToLower(a.Label)
		if strings.HasPrefix(l, "yes") || strings.HasPrefix(l, "approve") ||
			strings.HasPrefix(l, "accept") || strings.HasPrefix(l, "confirm") ||
			strings.HasPrefix(l, "allow") {
			return a, true
		}
	}
	for _, a := range answers {
		if a.Key == "y" {
			return a, true
		}
	}
	for _, a := range answers {
		if a.Key == "" { // press-Enter continue
			return a, true
		}
	}
	return Answer{}, false
}

// cleanLabel trims surrounding decoration and caps length for display.
func cleanLabel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, " .")
	if r := []rune(s); len(r) > 60 {
		s = string(r[:59]) + "…"
	}
	return s
}

// tailLines returns the last n non-empty lines in original (top-to-bottom) order.
func tailLines(s string, n int) []string {
	var nonEmpty []string
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) != "" {
			nonEmpty = append(nonEmpty, ln)
		}
	}
	if len(nonEmpty) > n {
		nonEmpty = nonEmpty[len(nonEmpty)-n:]
	}
	return nonEmpty
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
