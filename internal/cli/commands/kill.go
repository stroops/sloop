package commands

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/fleetstate"
	"github.com/stroops/sloop/internal/tmux"
)

// killFunc is a seam so RunKill can be tested without tmux.
var killFunc = tmux.Kill

// confirm prints prompt and returns true only on y/yes.
func confirm(w io.Writer, in io.Reader, prompt string) bool {
	_, _ = fmt.Fprint(w, prompt)
	line, _ := bufio.NewReader(in).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func rowNames(rows []FleetRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

// selectSessions resolves which sessions a fleet action refers to (pure):
// every row (--all), waiting rows (--waiting), or each explicit target. Shared
// by `kill` and `approve`.
func selectSessions(rows []FleetRow, targets []string, all, waiting bool) ([]FleetRow, error) {
	switch {
	case all:
		if len(rows) == 0 {
			return nil, fmt.Errorf("no running AI sessions")
		}
		return rows, nil
	case waiting:
		w := filterWaiting(rows)
		if len(w) == 0 {
			return nil, fmt.Errorf("no agents waiting on you")
		}
		return w, nil
	case len(targets) == 0:
		return nil, fmt.Errorf("specify a target (number/session/workspace), or --all / --waiting")
	default:
		var out []FleetRow
		for _, t := range targets {
			row, err := resolveTarget(rows, t)
			if err != nil {
				return nil, err
			}
			out = append(out, row)
		}
		return out, nil
	}
}

// RunKill ends the targeted session(s), confirming first unless yes.
func RunKill(w io.Writer, in io.Reader, targets []string, all, waiting, yes bool) ([]string, error) {
	if !tmux.Available() {
		return nil, fmt.Errorf("tmux is not installed; `sloop kill` needs tmux")
	}
	manifests, _ := adapter.Load()
	rows := enrichGlances(fleetRows(tmux.ParseSessions(tmuxList()), manifests), manifests)
	if len(rows) == 0 {
		rows = enrichGlances(rows, manifests) // need status to find who's waiting
	}
	victims, err := selectSessions(rows, targets, all, waiting)
	if err != nil {
		return nil, err
	}
	if !yes {
		if !confirm(w, in, fmt.Sprintf("kill %d session(s): %s? [y/N] ",
			len(victims), strings.Join(rowNames(victims), ", "))) {
			return nil, nil
		}
	}
	var killed []string
	for _, v := range victims {
		if err := killFunc(v.Name); err != nil {
			return killed, err
		}
		// The session is gone, so its status marker is garbage — clear it now
		// rather than waiting for the age-based prune.
		fleetstate.Remove(v.Name)
		killed = append(killed, v.Name)
	}
	return killed, nil
}

var (
	killAll     bool
	killWaiting bool
	killYes     bool
)

var killCmd = &cobra.Command{
	Use:   "kill [<#|session|workspace>...]",
	Short: "End running AI session(s); stops the agent (use -y to skip confirm)",
	RunE: func(cmd *cobra.Command, args []string) error {
		killed, err := RunKill(cmd.OutOrStdout(), cmd.InOrStdin(), args, killAll, killWaiting, assumeYes(cmd, killYes))
		if err != nil {
			return err
		}
		if len(killed) == 0 {
			cmd.Println("nothing killed")
			return nil
		}
		for _, k := range killed {
			cmd.Printf("killed %s\n", k)
		}
		return nil
	},
}

// assumeYes is true when a command's local --yes is set or the global -y/--auto
// flag is. Lets us reuse the existing global "assume yes" without a -y clash.
func assumeYes(cmd *cobra.Command, local bool) bool {
	if local {
		return true
	}
	auto, _ := cmd.Flags().GetBool("auto")
	return auto
}

func RegisterKill(cmd *cobra.Command) {
	killCmd.Flags().BoolVar(&killAll, "all", false, "kill every running session")
	killCmd.Flags().BoolVar(&killWaiting, "waiting", false, "kill sessions waiting on you")
	killCmd.Flags().BoolVar(&killYes, "yes", false, "skip the confirmation (or use global -y)")
	killCmd.ValidArgsFunction = completeSessionNames
	cmd.AddCommand(killCmd)
}
