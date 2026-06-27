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
	"github.com/stroops/sloop/internal/tui"
)

// FleetRow is one running sloop AI session, ready to display.
type FleetRow struct {
	Workspace string
	Tool      string
	Name      string
	Attached  bool
	Windows   int
	Activity  time.Time
	Glance    string // last line of the session's own terminal output (best-effort)
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

// enrichGlances fills each row's Glance with the last line of its terminal
// (best-effort; reads only your own panes, never the provider).
func enrichGlances(rows []FleetRow) []FleetRow {
	for i := range rows {
		out, err := exec.Command("tmux", runner.BuildTmuxCaptureArgs(rows[i].Name)...).Output()
		if err != nil {
			continue
		}
		rows[i].Glance = truncate(runner.LastNonEmptyLine(string(out)), 72)
	}
	return rows
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func RunPs(w io.Writer, rows []FleetRow) error {
	if len(rows) == 0 {
		fmt.Fprintln(w, "⚓ No running AI sessions. Start one with `sloop run <tool>`.")
		return nil
	}
	fmt.Fprintf(w, "⚓ AI fleet — %d running\n\n", len(rows))
	for i, r := range rows {
		state := "○ idle"
		if r.Attached {
			state = "● attached"
		}
		fmt.Fprintf(w, "  %-3d %-16s %-9s %s · %s\n",
			i+1, r.Workspace, r.Tool, state, humanizeSince(r.Activity))
		if r.Glance != "" {
			fmt.Fprintf(w, "      └ %s\n", r.Glance)
		}
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

		if len(rows) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "⚓ No running AI sessions. Start one with `sloop run <tool>`.")
			return nil
		}

		rows = enrichGlances(rows)

		var options []string
		for _, r := range rows {
			stateMark := "\033[32m🟢 idle\033[0m"
			if r.Attached {
				stateMark = "\033[34m🔵 attached\033[0m"
			}

			line := fmt.Sprintf("%-16s %-9s %s · %s", r.Workspace, r.Tool, stateMark, humanizeSince(r.Activity))
			if r.Glance != "" {
				line += fmt.Sprintf("\r\n       └ \033[90m%s\033[0m", r.Glance)
			}
			options = append(options, line)
		}

		prompt := fmt.Sprintf("⚓ AI fleet — %d running\r\nSelect a session to attach (↑/↓ to navigate, Enter to attach, Esc to quit):", len(rows))
		
		// Import tui dynamically since it's an internal package
		// The import will be added via goimports or explicitly
		// Actually, I'll need to make sure the import is there.
		
		selected, err := tui.SelectMenu(prompt, options)
		if err != nil {
			return err
		}
		if selected >= 0 {
			return jumpToFleet(rows, selected+1)
		}
		return nil
	},
}

func RegisterPs(cmd *cobra.Command) { cmd.AddCommand(psCmd) }
