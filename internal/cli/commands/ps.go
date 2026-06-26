package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/runner"
)

// FleetRow is one running sloop AI session, ready to display.
type FleetRow struct {
	Workspace string
	Tool      string
	Name      string
	Attached  bool
	Windows   int
	Activity  time.Time
}

// fleetRows keeps only sloop-named sessions (`<workspace>__<tool>`), splitting
// on the last `__`, and sorts them by workspace then tool.
func fleetRows(sessions []runner.TmuxSession) []FleetRow {
	var rows []FleetRow
	for _, s := range sessions {
		i := strings.LastIndex(s.Name, "__")
		if i < 0 {
			continue // not a sloop session
		}
		rows = append(rows, FleetRow{
			Workspace: s.Name[:i],
			Tool:      s.Name[i+2:],
			Name:      s.Name,
			Attached:  s.Attached,
			Windows:   s.Windows,
			Activity:  s.Activity,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Workspace != rows[j].Workspace {
			return rows[i].Workspace < rows[j].Workspace
		}
		return rows[i].Tool < rows[j].Tool
	})
	return rows
}

// tmuxList runs `tmux list-sessions`; an error (no server / no sessions) is
// treated as an empty fleet rather than a hard failure.
func tmuxList() string {
	out, err := exec.Command("tmux", runner.BuildTmuxListArgs()...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func RunPs(w io.Writer, rows []FleetRow) error {
	if len(rows) == 0 {
		fmt.Fprintln(w, "⚓ No running AI sessions. Start one with `sloop run <tool>`.")
		return nil
	}
	fmt.Fprintf(w, "⚓ AI fleet — %d running\n\n", len(rows))
	fmt.Fprintf(w, "  %-3s %-18s %-9s %-4s %s\n", "#", "workspace", "tool", "win", "state")
	for i, r := range rows {
		state := "○ idle"
		if r.Attached {
			state = "● attached"
		}
		fmt.Fprintf(w, "  %-3d %-18s %-9s %-4d %s · %s\n",
			i+1, r.Workspace, r.Tool, r.Windows, state, humanizeSince(r.Activity))
	}
	fmt.Fprintln(w, "\njump: sloop ps <#>   (switches client if you're already in tmux)")
	return nil
}

func humanizeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "active just now"
	case d < time.Hour:
		return fmt.Sprintf("active %dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("active %dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("active %dd ago", int(d.Hours())/24)
	}
}

// jumpToFleet attaches to row n (1-based); switches the client instead when
// already inside tmux, since attach cannot nest.
func jumpToFleet(rows []FleetRow, n int) error {
	if n < 1 || n > len(rows) {
		return fmt.Errorf("no session #%d (have %d)", n, len(rows))
	}
	if !runner.TmuxAvailable() {
		return fmt.Errorf("tmux is not installed")
	}
	name := rows[n-1].Name
	args := runner.BuildTmuxAttachArgs(name)
	if os.Getenv("TMUX") != "" {
		args = runner.BuildTmuxSwitchArgs(name)
	}
	cmd := exec.Command("tmux", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

var psCmd = &cobra.Command{
	Use:   "ps [#]",
	Short: "List running AI sessions (the fleet); `sloop ps <#>` jumps to one",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rows := fleetRows(runner.ParseSessions(tmuxList()))
		if len(args) == 1 {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("argument must be a session number: %q", args[0])
			}
			return jumpToFleet(rows, n)
		}
		return RunPs(cmd.OutOrStdout(), rows)
	},
}

func RegisterPs(cmd *cobra.Command) { cmd.AddCommand(psCmd) }
