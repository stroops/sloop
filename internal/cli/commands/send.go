package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/tmux"
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
	if !tmux.Available() {
		return fmt.Errorf("tmux is not installed; `sloop send` needs tmux")
	}
	rows := fleetRows(tmux.ParseSessions(tmuxList()))
	row, err := resolveTarget(rows, target)
	if err != nil {
		return err
	}
	return tmux.LaunchSend(row.Name, msg)
}

// RunSendBroadcast types msg into every session (all) or every waiting session,
// returning how many it reached.
func RunSendBroadcast(msg string, all, waiting bool) (int, error) {
	if strings.TrimSpace(msg) == "" {
		return 0, fmt.Errorf("nothing to send (empty message)")
	}
	if !tmux.Available() {
		return 0, fmt.Errorf("tmux is not installed; `sloop send` needs tmux")
	}
	manifests, _ := adapter.Load()
	rows := enrichGlances(fleetRows(tmux.ParseSessions(tmuxList())), manifests)
	if waiting {
		rows = filterWaiting(rows)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("no matching sessions")
	}
	n := 0
	for _, r := range rows {
		if err := tmux.LaunchSend(r.Name, msg); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

var (
	sendAll     bool
	sendWaiting bool
	sendYes     bool
)

var sendCmd = &cobra.Command{
	Use:   "send [<session|workspace|#>] <message...>",
	Short: "Send a prompt to a running session without attaching (via tmux)",
	Long: `Type a prompt into a running AI session without attaching to it.

The target is a fleet number (from ` + "`sloop ps`" + `), a full session name
(<workspace>__<tool>), or a workspace name when only one session runs in it.
With --waiting / --all, omit the target to broadcast to every waiting / running
session. send-keys types into your own pane exactly as if you typed it — the
provider is never intercepted.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if sendAll || sendWaiting {
			if len(args) < 1 {
				return fmt.Errorf("provide a message to broadcast")
			}
			return nil
		}
		if len(args) < 2 {
			return fmt.Errorf("requires <target> <message...> (or --waiting/--all <message>)")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if sendAll || sendWaiting {
			msg := strings.Join(args, " ")
			if sendAll && !assumeYes(cmd, sendYes) && !confirm(cmd.OutOrStdout(), cmd.InOrStdin(),
				"broadcast to ALL running sessions? [y/N] ") {
				return nil
			}
			n, err := RunSendBroadcast(msg, sendAll, sendWaiting)
			if err != nil {
				return err
			}
			cmd.Printf("sent to %d sessions\n", n)
			return nil
		}
		target := args[0]
		msg := strings.Join(args[1:], " ")
		if err := RunSend(target, msg); err != nil {
			return err
		}
		cmd.Printf("sent to %s\n", target)
		return nil
	},
}

func RegisterSend(cmd *cobra.Command) {
	sendCmd.Flags().BoolVar(&sendAll, "all", false, "broadcast to every running session")
	sendCmd.Flags().BoolVar(&sendWaiting, "waiting", false, "broadcast to every session waiting on you")
	sendCmd.Flags().BoolVar(&sendYes, "yes", false, "skip the --all confirmation (or use global -y)")
	sendCmd.ValidArgsFunction = completeSendTargets
	cmd.AddCommand(sendCmd)
}
