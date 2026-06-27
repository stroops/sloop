package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/runner"
)

// resolveTarget maps a user-supplied target to a fleet session. It accepts a
// 1-based fleet number (as shown by `sloop ps`), an exact session name
// (`<workspace>__<tool>`), or a workspace name when exactly one session runs in
// it. Ambiguous or unknown targets return an actionable error.
func resolveTarget(rows []FleetRow, target string) (FleetRow, error) {
	if len(rows) == 0 {
		return FleetRow{}, fmt.Errorf("no running AI sessions")
	}
	if n, err := strconv.Atoi(target); err == nil {
		if n < 1 || n > len(rows) {
			return FleetRow{}, fmt.Errorf("no session #%d (have %d)", n, len(rows))
		}
		return rows[n-1], nil
	}
	for _, r := range rows {
		if r.Name == target {
			return r, nil
		}
	}
	var matches []FleetRow
	for _, r := range rows {
		if r.Workspace == target {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return FleetRow{}, fmt.Errorf("no session matching %q; run `sloop ps` to list them", target)
	default:
		var names []string
		for _, m := range matches {
			names = append(names, m.Name)
		}
		return FleetRow{}, fmt.Errorf("%q is ambiguous (%s); pass the full session name or a number",
			target, strings.Join(names, ", "))
	}
}

// RunSend resolves target against the live fleet and types msg into its pane.
func RunSend(target, msg string) error {
	if strings.TrimSpace(msg) == "" {
		return fmt.Errorf("nothing to send (empty message)")
	}
	if !runner.TmuxAvailable() {
		return fmt.Errorf("tmux is not installed; `sloop send` needs tmux")
	}
	rows := fleetRows(runner.ParseSessions(tmuxList()))
	row, err := resolveTarget(rows, target)
	if err != nil {
		return err
	}
	return runner.LaunchSend(row.Name, msg)
}

var sendCmd = &cobra.Command{
	Use:   "send <session|workspace|#> <message...>",
	Short: "Send a prompt to a running session without attaching (via tmux)",
	Long: `Type a prompt into a running AI session without attaching to it.

The target is a fleet number (from ` + "`sloop ps`" + `), a full session name
(<workspace>__<tool>), or a workspace name when only one session runs in it.
send-keys types into your own pane exactly as if you typed it — the provider is
never intercepted.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		msg := strings.Join(args[1:], " ")
		if err := RunSend(target, msg); err != nil {
			return err
		}
		cmd.Printf("sent to %s\n", target)
		return nil
	},
}

func RegisterSend(cmd *cobra.Command) { cmd.AddCommand(sendCmd) }
