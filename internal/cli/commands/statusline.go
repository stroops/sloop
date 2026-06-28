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
	ws, tool := session, ""
	if i := strings.LastIndex(session, "__"); i >= 0 {
		ws, tool = session[:i], session[i+2:]
	}
	st := tmux.StatusUnknown
	if m, ok := fleetstate.Read(session); ok {
		st = stateToStatus(m.Status)
	} else if out, err := tmux.Output(tmux.BuildCaptureArgs(session)...); err == nil {
		manifests, _ := adapter.Load()
		st = tmux.ClassifyStatus(string(out), manifests[tool])
	}
	return fmt.Sprintf("⚓ %s %s %s", ws, tool, tmuxStatusLabel(st))
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
