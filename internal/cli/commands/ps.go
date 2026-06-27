package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/stroops/sloop/internal/fleetstate"
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
	Glance    string             // last line of the session's own terminal output (best-effort)
	Status    runner.AgentStatus // waiting / working / idle, classified from the pane
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

// captureTimeout bounds each `tmux capture-pane` so one hung pane can never
// hang the whole fleet view (important for --watch, which loops forever).
const captureTimeout = 2 * time.Second

// enrichGlances fills each row's Glance and Status from its terminal, capturing
// every pane concurrently so the fleet renders fast even with many sessions
// (best-effort; reads only your own panes, never the provider). Each goroutine
// writes a distinct index, so no locking is needed, and each capture is bounded
// by captureTimeout so a stuck pane can't block. When a fresh hook-written
// marker exists for a session it overrides the pane heuristic (authoritative).
func enrichGlances(rows []FleetRow) []FleetRow {
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			marker, hasMarker := fleetstate.Read(rows[i].Name)

			ctx, cancel := context.WithTimeout(context.Background(), captureTimeout)
			defer cancel()
			out, err := exec.CommandContext(ctx, "tmux", runner.BuildTmuxCaptureArgs(rows[i].Name)...).Output()
			if err == nil {
				rows[i].Glance = truncate(runner.LastNonEmptyLine(string(out)), 72)
				rows[i].Status = runner.ClassifyStatus(string(out))
			}
			if hasMarker {
				rows[i].Status = stateToStatus(marker.Status)
			}
		}(i)
	}
	wg.Wait()
	return rows
}

// stateToStatus maps a hook marker's status string to an AgentStatus.
func stateToStatus(s string) runner.AgentStatus {
	switch s {
	case "waiting":
		return runner.StatusWaiting
	case "working":
		return runner.StatusWorking
	case "idle":
		return runner.StatusIdle
	default:
		return runner.StatusUnknown
	}
}

// filterWaiting keeps only sessions waiting on the user.
func filterWaiting(rows []FleetRow) []FleetRow {
	var out []FleetRow
	for _, r := range rows {
		if r.Status.NeedsAttention() {
			out = append(out, r)
		}
	}
	return out
}

// newlyWaiting returns the names of sessions waiting now that were not waiting
// in the previous snapshot — the agents that just started needing you.
func newlyWaiting(prev, curr []FleetRow) []string {
	was := make(map[string]bool)
	for _, r := range prev {
		if r.Status.NeedsAttention() {
			was[r.Name] = true
		}
	}
	var out []string
	for _, r := range curr {
		if r.Status.NeedsAttention() && !was[r.Name] {
			out = append(out, r.Name)
		}
	}
	return out
}

// sortNeedsAttention floats sessions waiting on the user to the top, then keeps
// the stable workspace/tool order — so the agents that need you are listed first.
func sortNeedsAttention(rows []FleetRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Status.NeedsAttention() && !rows[j].Status.NeedsAttention()
	})
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
	waiting := 0
	for _, r := range rows {
		if r.Status.NeedsAttention() {
			waiting++
		}
	}
	header := fmt.Sprintf("⚓ AI fleet — %d running", len(rows))
	if waiting > 0 {
		header += fmt.Sprintf(", %d waiting on you", waiting)
	}
	fmt.Fprintf(w, "%s\n\n", header)
	for i, r := range rows {
		fmt.Fprintf(w, "  %-3d %-16s %-9s %s · %s\n",
			i+1, r.Workspace, r.Tool, stateLabel(r), humanizeSince(r.Activity))
		if r.Glance != "" {
			fmt.Fprintf(w, "      └ %s\n", r.Glance)
		}
	}
	fmt.Fprintln(w, "\njump: sloop ps <#>   ·   send: sloop send <#> \"msg\"")
	return nil
}

// stateLabel combines attach state with the classified agent status, leading
// with whichever matters most (a session waiting on you wins).
func stateLabel(r FleetRow) string {
	switch r.Status {
	case runner.StatusWaiting:
		return "◆ waiting on you"
	case runner.StatusWorking:
		return "▸ working"
	}
	if r.Attached {
		return "● attached"
	}
	return "○ idle"
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
	fmt.Printf("\n\033[36m💡 SLOOP HINT: To safely hide this agent and return to the terminal,\033[0m\n")
	fmt.Printf("\033[36m   press \033[1m%s\033[0m\033[36m then press \033[1md\033[0m\n\n", runner.TmuxPrefix())

	cmd := exec.Command("tmux", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

var (
	psWatch    bool
	psWaiting  bool
	psNotify   bool
	psInterval time.Duration
)

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

		if psWatch {
			return runWatch(cmd.OutOrStdout(), psInterval, psWaiting, psNotify)
		}

		if len(rows) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "⚓ No running AI sessions. Start one with `sloop run <tool>`.")
			return nil
		}

		rows = enrichGlances(rows)
		sortNeedsAttention(rows)
		if psWaiting {
			rows = filterWaiting(rows)
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "⚓ No agents waiting on you.")
				return nil
			}
		}

		// Non-interactive (piped, CI): print the plain listing instead of the
		// raw-mode menu, which can't run without a tty.
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return RunPs(cmd.OutOrStdout(), rows)
		}

		wsW, toolW, waiting := columnWidths(rows)

		var options []string
		for _, r := range rows {
			dot, label := statusDot(r)
			line := fmt.Sprintf("%s %-*s %-*s %-16s %s",
				dot, wsW, r.Workspace, toolW, r.Tool, label, shortSince(r.Activity))
			if r.Glance != "" {
				line += "\r\n└ " + tui.Grey(r.Glance)
			}
			options = append(options, line)
		}

		header := fmt.Sprintf("⚓ AI fleet · %d running", len(rows))
		if waiting > 0 {
			header += " · " + tui.Yellow(fmt.Sprintf("%d waiting on you", waiting))
		}
		prompt := header + "\r\n" + tui.Grey("  ↑/↓ move · ⏎ attach · q quit")

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

// statusDot renders the agent status as a colored dot plus a label for the
// interactive menu: waiting (needs you) yellow, working cyan, attached blue,
// idle green. A filled dot means active; a hollow dot means idle.
func statusDot(r FleetRow) (dot, label string) {
	switch r.Status {
	case runner.StatusWaiting:
		return tui.Yellow("●"), "waiting on you"
	case runner.StatusWorking:
		return tui.Cyan("●"), "working"
	}
	if r.Attached {
		return tui.Blue("●"), "attached"
	}
	return tui.Green("○"), "idle"
}

// shortSince is a compact relative time ("now", "3m", "2h", "5d").
func shortSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}

// columnWidths returns the workspace and tool column widths needed to keep the
// list aligned regardless of name length, plus the count of waiting sessions.
func columnWidths(rows []FleetRow) (wsW, toolW, waiting int) {
	for _, r := range rows {
		if len(r.Workspace) > wsW {
			wsW = len(r.Workspace)
		}
		if len(r.Tool) > toolW {
			toolW = len(r.Tool)
		}
		if r.Status.NeedsAttention() {
			waiting++
		}
	}
	return wsW, toolW, waiting
}

// runWatch re-renders the fleet on an interval (until Ctrl-C), ringing the
// terminal bell — and optionally a desktop notification — whenever a session
// newly starts waiting on you. This turns `ps` from a snapshot into a live
// monitor: you no longer have to keep re-running it to catch who needs you.
func runWatch(w io.Writer, interval time.Duration, waitingOnly, notify bool) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	var prev []FleetRow
	for {
		rows := enrichGlances(fleetRows(runner.ParseSessions(tmuxList())))
		sortNeedsAttention(rows)

		shown := rows
		if waitingOnly {
			shown = filterWaiting(rows)
		}

		fmt.Fprint(w, "\033[H\033[2J") // home + clear screen
		if waitingOnly && len(shown) == 0 {
			fmt.Fprintln(w, "⚓ No agents waiting on you.")
		} else {
			_ = RunPs(w, shown)
		}
		fmt.Fprintf(w, "\nwatching every %s · Ctrl-C to stop\n", interval)

		for _, name := range newlyWaiting(prev, rows) {
			fmt.Fprint(w, "\a") // bell
			if notify {
				runner.Notify("sloop", name+" is waiting on you")
			}
		}
		prev = rows
		time.Sleep(interval)
	}
}

func RegisterPs(cmd *cobra.Command) {
	psCmd.Flags().BoolVarP(&psWatch, "watch", "f", false, "follow the fleet live: refresh on an interval and alert when an agent needs you")
	psCmd.Flags().BoolVar(&psWaiting, "waiting", false, "show only sessions waiting on you")
	psCmd.Flags().BoolVar(&psNotify, "notify", false, "with --watch, also send a desktop notification on new waiting agents")
	psCmd.Flags().DurationVarP(&psInterval, "interval", "n", 2*time.Second, "refresh interval for --watch")
	psCmd.ValidArgsFunction = completePsIndex
	cmd.AddCommand(psCmd)
}
