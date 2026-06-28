package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/fleetstate"
	"github.com/stroops/sloop/internal/tmux"
)

// tmuxStatusLabel renders an agent status with tmux's own color format
// (`#[fg=…]…#[default]`), kept short and clear for the status bar.
func tmuxStatusLabel(s tmux.AgentStatus) string {
	switch s {
	case tmux.StatusWaiting:
		return "#[fg=yellow]◆ waiting#[default]"
	case tmux.StatusWorking:
		return "#[fg=cyan]▸ working#[default]"
	case tmux.StatusIdle:
		return "#[fg=green]○ idle#[default]"
	default:
		return "○"
	}
}

// renderStatusline builds the one-line status bar text for a session, e.g.
// `⚓ myrepo claude ◆ waiting`. Prefers a fresh hook marker over the heuristic.
func renderStatusline(session string) string {
	if session == "" {
		return ""
	}
	manifests, _ := adapter.Load()
	ws, tool, instance := splitSession(session, manifests)
	label := tool
	if instance != "" {
		label = tool + "·" + instance // distinguish a second agent of the same tool
	}
	st := tmux.StatusUnknown
	if m, ok := fleetstate.Read(session); ok {
		st = stateToStatus(m.Status)
	} else if out, err := tmux.Output(tmux.BuildCaptureArgs(session)...); err == nil {
		st = tmux.ClassifyStatus(string(out), manifests[tool])
	}
	return fmt.Sprintf("⚓ %s %s %s%s", ws, label, tmuxStatusLabel(st), renderFleetBadge(session))
}

// waitingBadge formats the fleet-wide waiting count for the status bar, empty
// when nothing is waiting so the bar stays clean.
func waitingBadge(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(" #[fg=yellow]⏳ %d waiting#[default]", n)
}

// renderFleetBadge counts sloop sessions whose fresh marker is `waiting`,
// excluding the current session (so it reads "others need you"). Markers only —
// no capture-pane — because this re-renders every status-interval.
func renderFleetBadge(exclude string) string {
	n := 0
	for _, s := range tmux.ParseSessions(tmuxList()) {
		if s.Name == exclude {
			continue
		}
		if strings.LastIndex(s.Name, "__") < 0 {
			continue
		}
		if m, ok := fleetstate.Read(s.Name); ok && m.Status == "waiting" {
			n++
		}
	}
	return waitingBadge(n)
}

var statuslineCmd = &cobra.Command{
	Use:    "statusline [session]",
	Short:  "Render a session's live status for the tmux status bar",
	Hidden: true, // called by tmux via #(), not by users directly
	Args:   cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		session := currentSession()
		if len(args) == 1 {
			session = args[0]
		}
		// Must go to stdout: tmux's #() in the status bar captures stdout only.
		// (cobra's cmd.Print writes to stderr, which the status bar never sees.)
		_, _ = fmt.Fprint(cmd.OutOrStdout(), renderStatusline(session))
		return nil
	},
}

var statuslineSetupCmd = &cobra.Command{
	Use:   "setup [session]",
	Short: "Show sloop's status bar in a session (default: the current one)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tmux.Available() {
			return fmt.Errorf("`sloop statusline setup` needs tmux")
		}
		session := currentSession()
		if len(args) == 1 {
			session = args[0]
		}
		if session == "" {
			return fmt.Errorf("run inside a tmux session, or pass a session name")
		}
		tmux.SetStatusLine(session)
		cmd.Printf("status bar enabled for %s\n", session)
		return nil
	},
}

func RegisterStatusline(cmd *cobra.Command) {
	statuslineCmd.AddCommand(statuslineSetupCmd)
	statuslineCmd.ValidArgsFunction = completeSessionNames
	cmd.AddCommand(statuslineCmd)
}
