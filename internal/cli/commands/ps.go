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

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/fleetstate"
	"github.com/stroops/sloop/internal/hints"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
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
	Glance    string           // last line of the session's own terminal output (best-effort)
	Status    tmux.AgentStatus // waiting / working / idle, classified from the pane
	Path      string           // repo path from the registry (cross-repo context)
	Prompt    string           // the question the agent is blocked on (when waiting)
	Answers   []tmux.Answer    // parsed choices the agent offers (answer in one key)
	Display   string           // provider display name (e.g. "Google Antigravity" for "agy")
	Instance  string           // named instance suffix (e.g. "sec" / "2"); "" = the default session
}

// toolName is the row's provider display name, falling back to the tool key when
// the row hasn't been enriched (e.g. plain listings built straight from tmux).
// A named instance is appended (`claude·sec`) so two agents of the same provider
// in one workspace are distinguishable.
func (r FleetRow) toolName() string {
	name := r.Tool
	if r.Display != "" {
		name = r.Display
	}
	if r.Instance != "" {
		return name + "·" + r.Instance
	}
	return name
}

// registryPaths maps each registered workspace name to its repo path, so the
// fleet view can show where a session lives and surface known-but-idle repos.
func registryPaths() map[string]string {
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return nil
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	wss, err := store.ListWorkspaces()
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(wss))
	for _, ws := range wss {
		m[ws.Name] = ws.Path
	}
	return m
}

// annotatePaths fills each row's Path from the registry (best-effort).
func annotatePaths(rows []FleetRow, paths map[string]string) {
	for i := range rows {
		rows[i].Path = paths[rows[i].Workspace]
	}
}

// splitSession parses a sloop session name into workspace, tool, and instance.
// It is manifest-aware so the tool is found by key rather than by position,
// which keeps `ws__tool__instance` unambiguous: the tool is the last `__`
// segment that is a known tool (legacy `ws__tool` → instance ""), else the
// second-to-last when that is a known tool (→ instance = the last segment).
// With nil/empty manifests it falls back to the legacy last-`__` split, so
// callers that only need the name behave exactly as before.
func splitSession(name string, manifests map[string]adapter.Manifest) (ws, tool, instance string) {
	parts := strings.Split(name, "__")
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if _, ok := manifests[last]; ok {
			return strings.Join(parts[:len(parts)-1], "__"), last, ""
		}
		if len(parts) >= 3 {
			if mid := parts[len(parts)-2]; hasKey(manifests, mid) {
				return strings.Join(parts[:len(parts)-2], "__"), mid, last
			}
		}
	}
	i := strings.LastIndex(name, "__")
	if i < 0 {
		return name, "", "" // not a sloop session name
	}
	return name[:i], name[i+2:], ""
}

func hasKey(m map[string]adapter.Manifest, k string) bool {
	_, ok := m[k]
	return ok
}

// fleetRows keeps only sloop-named sessions (containing `__`), splitting each
// into workspace/tool/instance, and sorts them by workspace, tool, instance.
func fleetRows(sessions []tmux.Session, manifests map[string]adapter.Manifest) []FleetRow {
	var rows []FleetRow
	for _, s := range sessions {
		if !strings.Contains(s.Name, "__") {
			continue // not a sloop session
		}
		ws, tool, instance := splitSession(s.Name, manifests)
		rows = append(rows, FleetRow{
			Workspace: ws,
			Tool:      tool,
			Instance:  instance,
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
		if rows[i].Tool != rows[j].Tool {
			return rows[i].Tool < rows[j].Tool
		}
		return rows[i].Instance < rows[j].Instance
	})
	return rows
}

// tmuxList runs `tmux list-sessions`; an error (no server / no sessions) is
// treated as an empty fleet rather than a hard failure.
func tmuxList() string {
	out, err := tmux.Output(tmux.BuildListArgs()...)
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
func enrichGlances(rows []FleetRow, manifests map[string]adapter.Manifest) []FleetRow {
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			marker, hasMarker := fleetstate.Read(rows[i].Name)
			rows[i].Display = displayTool(rows[i].Tool, manifests)

			ctx, cancel := context.WithTimeout(context.Background(), captureTimeout)
			defer cancel()
			out, err := tmux.OutputContext(ctx, tmux.BuildCaptureArgs(rows[i].Name)...)
			if err == nil {
				rows[i].Glance = truncate(tmux.LastNonEmptyLine(string(out)), 72)
				rows[i].Status = tmux.ClassifyStatus(string(out), manifests[rows[i].Tool])
			}
			if hasMarker {
				rows[i].Status = stateToStatus(marker.Status)
			}
			// When the agent is waiting, read what it's asking + the choices.
			if rows[i].Status == tmux.StatusWaiting && err == nil {
				rows[i].Prompt = tmux.PromptLine(string(out))
				rows[i].Answers = tmux.ParseAnswers(string(out))
			}
		}(i)
	}
	wg.Wait()
	return rows
}

// stateToStatus maps a hook marker's status string to an AgentStatus.
func stateToStatus(s string) tmux.AgentStatus {
	switch s {
	case "waiting":
		return tmux.StatusWaiting
	case "working":
		return tmux.StatusWorking
	case "idle":
		return tmux.StatusIdle
	default:
		return tmux.StatusUnknown
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
		_, _ = fmt.Fprintln(w, "⚓ No running AI sessions. Start one with `sloop run <tool>`.")
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
	_, _ = fmt.Fprintf(w, "%s\n\n", header)
	for i, r := range rows {
		_, _ = fmt.Fprintf(w, "  %-3d %-16s %-18s %s · %s\n",
			i+1, r.Workspace, r.toolName(), stateLabel(r), humanizeSince(r.Activity))
		if b := bottomLine(r); b != "" {
			_, _ = fmt.Fprintf(w, "      └ %s\n", b)
		}
	}
	_, _ = fmt.Fprintf(w, "\njump: sloop ps <#>   ·   send: sloop send <#> \"msg\"   ·   %s\n", tmux.DetachLine())
	return nil
}

// stateLabel combines attach state with the classified agent status, leading
// with whichever matters most (a session waiting on you wins).
func stateLabel(r FleetRow) string {
	switch r.Status {
	case tmux.StatusWaiting:
		return "◆ waiting on you"
	case tmux.StatusWorking:
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
	return attachSession(rows[n-1].Name)
}

// pickFleetSession shows the live fleet and returns the chosen session name ("" if
// cancelled). Backs the no-argument `sloop attach` so you don't have to type a
// session name — using the same WORKSPACE/TOOL/STATUS columns, colors and
// waiting-first order as `ps`, so the picker reads identically to the fleet view.
func pickFleetSession(title string) (string, error) {
	manifests, _ := adapter.Load()
	rows := enrichGlances(fleetRows(tmux.ParseSessions(tmuxList()), manifests), manifests)
	if len(rows) == 0 {
		return "", fmt.Errorf("no running AI sessions to attach to (start one with `sloop run <tool>`)")
	}
	sortNeedsAttention(rows)

	wsW, _, _ := columnWidths(rows)
	if wsW < len("WORKSPACE") {
		wsW = len("WORKSPACE")
	}
	toolW := len("TOOL")
	for _, r := range rows {
		if n := len(r.toolName()); n > toolW {
			toolW = n
		}
	}
	opts := make([]string, len(rows))
	for i, r := range rows {
		opts[i] = fmt.Sprintf("%-*s %-*s %s", wsW, r.Workspace, toolW, r.toolName(), statusText(r))
	}

	legend := "  " + tui.Yellow("waiting") + " · " + tui.Cyan("working") + " · " +
		tui.Blue("attached") + " · " + tui.Green("idle")
	cols := tui.Grey(fmt.Sprintf("  %-*s %-*s %s", wsW, "WORKSPACE", toolW, "TOOL", "STATUS"))
	tui.Clear()
	idx, err := tui.SelectMenu(title+"\r\n"+legend+"\r\n\r\n"+cols, opts)
	if err != nil || idx < 0 {
		return "", err
	}
	return rows[idx].Name, nil
}

// attachSession attaches to a session by name (switches client if already inside
// tmux), replacing this process's foreground with tmux. Shared by `ps` and `ls`.
func attachSession(name string) error {
	if !tmux.Available() {
		return fmt.Errorf("tmux is not installed")
	}
	args := tmux.BuildAttachArgs(name)
	if os.Getenv("TMUX") != "" {
		args = tmux.BuildSwitchArgs(name)
	}
	fmt.Printf("\n%s\n\n", tmux.DetachHint())

	cmd := exec.Command(tmux.Bin(), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

var (
	psWatch    bool
	psWaiting  bool
	psNotify   bool
	psAll      bool
	psInterval time.Duration
)

// notRunningWorkspaces lists registered workspaces that have no live session,
// sorted by name — the rest of your cross-repo fleet that isn't running yet.
func notRunningWorkspaces(rows []FleetRow, paths map[string]string) []string {
	running := make(map[string]bool, len(rows))
	for _, r := range rows {
		running[r.Workspace] = true
	}
	var idle []string
	for name := range paths {
		if !running[name] {
			idle = append(idle, name)
		}
	}
	sort.Strings(idle)
	return idle
}

// externalSessions returns live tmux sessions NOT created by sloop (no "__"),
// so the fleet can surface agents you started yourself and offer to adopt them.
func externalSessions(sessions []tmux.Session) []tmux.Session {
	var out []tmux.Session
	for _, s := range sessions {
		if !strings.Contains(s.Name, "__") {
			out = append(out, s)
		}
	}
	return out
}

// externalNudge prints a one-line pointer to unmanaged tmux sessions.
func externalNudge(w io.Writer, ext []tmux.Session) {
	if len(ext) == 0 {
		return
	}
	names := make([]string, 0, len(ext))
	for _, s := range ext {
		names = append(names, s.Name)
	}
	_, _ = fmt.Fprintln(w, tui.Grey(fmt.Sprintf("+ %d tmux session(s) not in sloop (%s) — `sloop adopt <name>` to add",
		len(ext), strings.Join(names, ", "))))
}

// runPsAll prints the live fleet, the registered workspaces that aren't running,
// and any unmanaged tmux sessions — the full picture, not just what sloop runs.
func runPsAll(w io.Writer, rows []FleetRow, paths map[string]string, ext []tmux.Session) error {
	_ = RunPs(w, rows)
	if idle := notRunningWorkspaces(rows, paths); len(idle) > 0 {
		_, _ = fmt.Fprintf(w, "\nKnown workspaces (not running):\n")
		for _, name := range idle {
			_, _ = fmt.Fprintf(w, "  ○ %-16s %s\n", name, paths[name])
		}
		_, _ = fmt.Fprintln(w, "\nstart one: sloop run -w <name>")
	}
	if len(ext) > 0 {
		_, _ = fmt.Fprintf(w, "\nOther tmux sessions (not managed by sloop):\n")
		for _, s := range ext {
			_, _ = fmt.Fprintf(w, "  ◌ %s\n", s.Name)
		}
		_, _ = fmt.Fprintln(w, "\nadopt one: sloop adopt <name>")
	}
	return nil
}

var psCmd = &cobra.Command{
	Use:   "ps [#]",
	Short: "List running AI sessions (the fleet); `sloop ps <#>` jumps to one",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifests, _ := adapter.Load()
		parsed := tmux.ParseSessions(tmuxList())
		rows := fleetRows(parsed, manifests)
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

		rows = enrichGlances(rows, manifests)
		sortNeedsAttention(rows)
		paths := registryPaths()
		annotatePaths(rows, paths)
		ext := externalSessions(parsed)

		// --all: the full cross-repo board (live + known-but-idle + unmanaged).
		if psAll {
			return runPsAll(cmd.OutOrStdout(), rows, paths, ext)
		}

		// Non-interactive (piped, CI): print the plain listing instead of the
		// raw-mode control center, which can't run without a tty.
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			if psWaiting {
				rows = filterWaiting(rows)
			}
			if len(rows) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), emptyFleetMessage(psWaiting))
				externalNudge(cmd.OutOrStdout(), ext)
				return nil
			}
			if err := RunPs(cmd.OutOrStdout(), rows); err != nil {
				return err
			}
			externalNudge(cmd.OutOrStdout(), ext)
			hints.Show(cmd.OutOrStdout(), "ps")
			return nil
		}

		// Interactive control center: loop so an action (send/answer/kill) or
		// cancelling returns to the fleet instead of dropping to the shell. The
		// screen is cleared each pass and feedback shows in `notice` under the
		// header, so the fleet redraws in place; it's re-read every pass, so kills
		// and new sessions show immediately.
		var notice string
		detach := tui.Grey("  ⏎ enters an agent — to come back, detach (keeps it running): " + tmux.PrefixRaw() + " d")
		for {
			tui.Clear()
			rows = enrichGlances(fleetRows(tmux.ParseSessions(tmuxList()), manifests), manifests)
			sortNeedsAttention(rows)
			annotatePaths(rows, registryPaths())
			if psWaiting {
				rows = filterWaiting(rows)
			}
			if len(rows) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), emptyFleetMessage(psWaiting))
				break
			}

			wsW, _, waiting := columnWidths(rows)
			toolW := len("TOOL")
			for _, r := range rows {
				if n := len(r.toolName()); n > toolW {
					toolW = n
				}
			}
			if wsW < len("WORKSPACE") {
				wsW = len("WORKSPACE")
			}
			var options []string
			for _, r := range rows {
				// No leading status glyph: ●/○ are ambiguous-width and break column
				// alignment. Status is the colored STATUS column instead.
				line := fmt.Sprintf("%-*s %-*s %s %s",
					wsW, r.Workspace, toolW, r.toolName(), statusText(r), shortSince(r.Activity))
				if b := bottomLine(r); b != "" {
					line += "\r\n└ " + tui.Grey(b)
				}
				options = append(options, line)
			}

			header := fmt.Sprintf("⚓ AI fleet · %d running", len(rows))
			if waiting > 0 {
				header += " · " + tui.Yellow(fmt.Sprintf("%d waiting on you", waiting))
			}
			legend := "  " + tui.Yellow("waiting") + " · " + tui.Cyan("working") + " · " +
				tui.Blue("attached") + " · " + tui.Green("idle") + tui.Grey(" · AGE = since last activity")
			keys := tui.Grey("  ↑/↓ move · ⏎ attach · 1/y answer · s send · x kill · q/esc quit")
			cols := tui.Grey(fmt.Sprintf("  %-*s %-*s %-16s %s", wsW, "WORKSPACE", toolW, "TOOL", "STATUS", "AGE"))
			prompt := header
			if notice != "" {
				prompt += "\r\n" + notice
			}
			prompt += "\r\n" + legend + "\r\n" + keys + "\r\n" + detach + "\r\n\r\n" + cols
			notice = "" // consumed; the handler below sets a fresh one for next pass

			actionKeys := []byte{'s', 'x', 'y', 'n', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
			idx, key, err := tui.SelectAction(prompt, options, actionKeys)
			if err != nil {
				return err
			}
			switch key {
			case 0: // q / Esc / Ctrl-C → leave the control center
				externalNudge(cmd.OutOrStdout(), ext)
				hints.Show(cmd.OutOrStdout(), "ps")
				return nil
			case 13: // Enter → attach (replaces this process)
				return jumpToFleet(rows, idx+1)
			case 's': // send a quick prompt; empty cancels back to the fleet
				n, err := promptAndSend(cmd, rows[idx])
				if err != nil {
					return err
				}
				notice = n
			case 'x': // kill the highlighted session (with confirm)
				n, err := confirmAndKill(cmd, rows[idx])
				if err != nil {
					return err
				}
				notice = n
			default:
				if a, ok := matchAnswer(rows[idx], key); ok { // one-key answer
					n, err := sendAnswer(cmd, rows[idx], a)
					if err != nil {
						return err
					}
					notice = n
				}
			}
		}
		externalNudge(cmd.OutOrStdout(), ext)
		hints.Show(cmd.OutOrStdout(), "ps")
		return nil
	},
}

// answerHint renders parsed choices as "[y]es [n]o" / "[1]Yes [2]No" / "[⏎]continue".
func answerHint(answers []tmux.Answer) string {
	if len(answers) == 0 {
		return ""
	}
	parts := make([]string, 0, len(answers))
	for _, a := range answers {
		key := a.Key
		if key == "" {
			key = "⏎"
		}
		parts = append(parts, "["+key+"]"+a.Label)
	}
	return strings.Join(parts, " ")
}

// bottomLine is the indented detail under a fleet row: the agent's question +
// answer keys when waiting, else the last output glance.
func bottomLine(r FleetRow) string {
	if r.Status == tmux.StatusWaiting && (r.Prompt != "" || len(r.Answers) > 0) {
		s := r.Prompt
		if h := answerHint(r.Answers); h != "" {
			if s != "" {
				s += "  ·  "
			}
			s += "answer: " + h
		}
		return s
	}
	return r.Glance
}

// matchAnswer returns the row's Answer whose Key equals the pressed key.
func matchAnswer(r FleetRow, key byte) (tmux.Answer, bool) {
	for _, a := range r.Answers {
		if a.Key == string(key) {
			return a, true
		}
	}
	return tmux.Answer{}, false
}

// sendAnswer types the chosen answer into the session, returning a notice line
// for the control center to show on its next redraw.
func sendAnswer(_ *cobra.Command, row FleetRow, a tmux.Answer) (string, error) {
	if err := tmux.LaunchSend(row.Name, a.Key); err != nil {
		return "", err
	}
	return tui.Green(fmt.Sprintf("✓ answered %s: %s", row.Name, a.Label)), nil
}

// promptAndSend asks for a line and sends it to the row (used by the ps `s` key).
// Reading in raw mode means Esc/Ctrl-C cancel back to the fleet instead of
// killing `ps`, and an empty submit cancels too. Returns a notice line ("" when
// cancelled) for the control center to show on its next redraw.
func promptAndSend(_ *cobra.Command, row FleetRow) (string, error) {
	msg, ok := tui.ReadLine(fmt.Sprintf("send to %s (Enter to send · Esc to cancel): ", row.Name))
	if !ok || strings.TrimSpace(msg) == "" {
		return "", nil
	}
	if err := tmux.LaunchSend(row.Name, msg); err != nil {
		return "", err
	}
	return tui.Green("✓ sent to " + row.Name), nil
}

// confirmAndKill ends the row's session after a y/N confirm (used by ps `x`).
// Returns a notice line ("" when declined) for the next redraw.
func confirmAndKill(_ *cobra.Command, row FleetRow) (string, error) {
	if !tui.Confirm(fmt.Sprintf("kill %s? [y/N] ", row.Name)) {
		return "", nil
	}
	if err := killFunc(row.Name); err != nil {
		return "", err
	}
	return tui.Yellow("✓ killed " + row.Name), nil
}

// displayTool returns a friendly name for a tool key (the manifest's display
// name, e.g. "Google Antigravity" for "agy"), falling back to the key for
// unknown/adopted tools.
func displayTool(key string, manifests map[string]adapter.Manifest) string {
	if m, ok := manifests[key]; ok && m.Name != "" {
		return m.Name
	}
	return key
}

// emptyFleetMessage is the "nothing here" line for ps, tailored to --waiting.
func emptyFleetMessage(waitingOnly bool) string {
	if waitingOnly {
		return "⚓ No agents waiting on you."
	}
	return "⚓ No running AI sessions. Start one with `sloop run <tool>` (or `sloop ps --all` to see known workspaces)."
}

// statusDot renders the agent status as a colored dot plus a label for the
// interactive menu: waiting (needs you) yellow, working cyan, attached blue,
// idle green. A filled dot means active; a hollow dot means idle.
func statusDot(r FleetRow) (dot, label string) {
	switch r.Status {
	case tmux.StatusWaiting:
		return tui.Yellow("●"), "waiting on you"
	case tmux.StatusWorking:
		return tui.Cyan("●"), "working"
	}
	if r.Attached {
		return tui.Blue("●"), "attached"
	}
	return tui.Green("○"), "idle"
}

// statusText is the colored, fixed-width STATUS column for the ps control
// center: the status as legible text (no ambiguous-width glyph), padded to 16
// before coloring so the trailing AGE column stays aligned.
func statusText(r FleetRow) string {
	pad := func(s string) string { return fmt.Sprintf("%-16s", s) }
	switch r.Status {
	case tmux.StatusWaiting:
		return tui.Yellow(pad("waiting on you"))
	case tmux.StatusWorking:
		return tui.Cyan(pad("working"))
	}
	if r.Attached {
		return tui.Blue(pad("attached"))
	}
	return tui.Green(pad("idle"))
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
		manifests, _ := adapter.Load()
		rows := enrichGlances(fleetRows(tmux.ParseSessions(tmuxList()), manifests), manifests)
		sortNeedsAttention(rows)

		shown := rows
		if waitingOnly {
			shown = filterWaiting(rows)
		}

		_, _ = fmt.Fprint(w, "\033[H\033[2J") // home + clear screen
		if waitingOnly && len(shown) == 0 {
			_, _ = fmt.Fprintln(w, "⚓ No agents waiting on you.")
		} else {
			_ = RunPs(w, shown)
		}
		_, _ = fmt.Fprintf(w, "\nwatching every %s · Ctrl-C to stop\n", interval)

		for _, name := range newlyWaiting(prev, rows) {
			_, _ = fmt.Fprint(w, "\a") // bell
			if notify {
				osNotify("sloop", name+" is waiting on you")
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
	psCmd.Flags().BoolVar(&psAll, "all", false, "also list registered workspaces that aren't running (full cross-repo board)")
	psCmd.Flags().DurationVarP(&psInterval, "interval", "n", 2*time.Second, "refresh interval for --watch")
	psCmd.ValidArgsFunction = completePsIndex
	cmd.AddCommand(psCmd)
}
