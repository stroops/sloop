package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/detect"
	"github.com/stroops/sloop/internal/tui"
	"github.com/stroops/sloop/internal/update"
	"github.com/stroops/sloop/internal/workspace"
)

// homeEntry is one row of the bare-`sloop` menu: a label, a one-line hint, and
// the subcommand it routes to (empty name = quit).
type homeEntry struct {
	label, hint, cmd string
}

// homeEntries is the menu shown when sloop is run with no arguments. It is a
// launcher over existing subcommands, not new behavior, so each row maps to a
// command a user could also type directly.
var homeEntries = []homeEntry{
	{"Fleet", "see running agents and jump into one", "ps"},
	{"Run", "start an AI session here", "run"},
	{"Sync", "sync context and skills across repos", "sync"},
	{"Check", "score this repo's AI-readiness", "check"},
	{"Init", "set up sloop in this repo", "init"},
	{"Doctor", "check tools, tmux, and config", "doctor"},
	{"More", "every command, with a one-line description", moreCmd},
	{"Quit", "", ""},
}

// moreCmd is a sentinel cmd value (not a real subcommand) marking the row that
// opens the full command reference.
const moreCmd = "\x00more"

// homeBanner returns the small masthead drawn above the menu: a wordmark, a
// rule, and a tagline carrying the version and (when present) the update notice.
// The rule gives the layout a fixed visual width so the menu below feels
// anchored rather than top-light.
func homeBanner() string {
	const rule = "  ────────────────────────────────"

	tagline := "  Navigate your AI fleet"
	if version != "" {
		tagline += " · " + version
	}

	lines := []string{
		tui.Cyan("  ⚓ ") + tui.Bold("s l o o p"),
		tui.Grey(rule),
		tui.Grey(tagline),
		contextLine(),
	}
	if banner := update.Banner(version); banner != "" {
		lines = append(lines, "  "+banner)
	}
	return strings.Join(lines, "\r\n")
}

// contextLine shows the directory sloop will act in — agents inherit the cwd as
// their project root, so making it visible avoids launching in the wrong place —
// plus whether sloop is initialized there (the dir is inside a .sloop workspace).
func contextLine() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	status := tui.Grey("· not initialized")
	if _, ok := workspaceHere(cwd); ok {
		status = tui.Green("✓ initialized")
	}
	return tui.Grey("  📂 "+tui.AbbreviateHome(cwd)) + "  " + status
}

// workspaceHere resolves the sloop workspace cwd sits in, but treats the global
// ~/.sloop config directory as "not a workspace". Without this, workspace.Resolve
// — which walks up looking for any .sloop — reports $HOME as a workspace for every
// directory under it, so an uninitialized folder wrongly shows as initialized.
func workspaceHere(cwd string) (*workspace.Workspace, bool) {
	ws, err := workspace.Resolve(cwd)
	if err != nil {
		return nil, false
	}
	if gdir, err := config.GlobalDir(); err == nil && ws.SloopDir() == gdir {
		return nil, false
	}
	return ws, true
}

// runHome shows the interactive launcher. It only takes over when sloop is run
// bare on a real terminal and the user hasn't opted out (SLOOP_NO_MENU); in any
// other case (piped, CI, --no-input) it prints help, preserving prior behavior.
func runHome(cmd *cobra.Command, _ []string) error {
	noInput, _ := cmd.Flags().GetBool("no-input")
	if os.Getenv("SLOOP_NO_MENU") != "" || noInput ||
		!term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return cmd.Help()
	}

	// Refresh the update cache out of band so the banner is current next time;
	// the banner shown now reflects a previous check (never blocks this one).
	update.MaybeTriggerBackground(version)

	options := make([]string, len(homeEntries))
	for i, e := range homeEntries {
		if e.hint == "" {
			options[i] = e.label
			continue
		}
		options[i] = fmt.Sprintf("%-7s %s", e.label, tui.Grey(e.hint))
	}

	// Loop so backing out of the More screen returns here rather than to the
	// shell; running a command (or Quit) breaks out.
	for {
		idx, key, err := tui.Menu{
			Prompt:     homeBanner(),
			Footer:     tui.Grey("  ↑/↓ move · ⏎ select · m more · q quit"),
			Options:    options,
			ActionKeys: []byte{'m'},
			Highlight:  tui.HighlightRow,
			TopPad:     true,
		}.Run()
		if err != nil || idx < 0 {
			return err
		}

		name := homeEntries[idx].cmd
		if key == 'm' { // shortcut: jump straight to the command reference
			name = moreCmd
		}
		switch name {
		case "": // Quit
			return nil
		case moreCmd:
			if ran, err := showMore(cmd.Root()); err != nil || ran {
				return err // a command was launched (or errored); otherwise loop back
			}
		case "run":
			// "Run" doesn't launch blindly: like the fleet board lists agents, Run
			// lets you pick which tool or profile to start (mirroring how Mole picks
			// a target before acting), not silently the workspace default.
			return runFromMenu(cmd.Root())
		default:
			return dispatch(cmd.Root(), name)
		}
	}
}

// showMore lists every command with its one-line description and runs the one
// picked. It returns ran=true when a command was dispatched (the caller is
// done), or false when the user backed out (q/Esc) so the home menu reappears.
func showMore(root *cobra.Command) (ran bool, err error) {
	cmds := referenceCommands(root)
	width := 0
	for _, c := range cmds {
		if n := len(c.Name()); n > width {
			width = n
		}
	}
	options := make([]string, len(cmds))
	for i, c := range cmds {
		options[i] = fmt.Sprintf("%-*s  %s", width, c.Name(), tui.Grey(c.Short))
	}

	idx, _, err := tui.Menu{
		Prompt:    tui.Cyan("  ⚓ ") + tui.Bold("All commands") + tui.Grey("  · ⏎ runs it (some need arguments — see -h)"),
		Footer:    tui.Grey("  ↑/↓ move · ⏎ run · q back"),
		Options:   options,
		Highlight: tui.HighlightRow,
		TopPad:    true,
	}.Run()
	if err != nil || idx < 0 {
		return false, err
	}
	return true, dispatch(root, cmds[idx].Name())
}

// referenceCommands returns the user-facing subcommands for the More screen:
// every registered command except hidden ones and cobra's auto-added help. The
// fleet views (ps, ls) are surfaced first since they're the most-used; the rest
// stay in cobra's alphabetical order.
func referenceCommands(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, c := range root.Commands() {
		if c.Hidden || !c.IsAvailableCommand() || c.Name() == "help" {
			continue
		}
		out = append(out, c)
	}
	priority := map[string]int{"ps": 0, "ls": 1}
	sort.SliceStable(out, func(i, j int) bool {
		pi, oki := priority[out[i].Name()]
		pj, okj := priority[out[j].Name()]
		if oki && okj {
			return pi < pj
		}
		return oki && !okj // prioritized commands first; others keep their order
	})
	return out
}

// runFromMenu picks a tool/profile and launches it. If the current directory
// isn't a sloop workspace yet — which AI CLIs treat as the project root, and
// where `sloop run` would otherwise fail — it offers to initialize it for the
// chosen tool first, then runs.
func runFromMenu(root *cobra.Command) error {
	args, ok := runMenu()
	if !ok { // cancelled the picker
		return nil
	}
	target := args[1]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, ok := workspaceHere(cwd); !ok {
		tool := target
		if strings.HasPrefix(tool, "@") {
			tool = profileTool(tool) // initialize for the profile's underlying tool
		}
		prompt := fmt.Sprintf("%s isn't set up for sloop yet. Initialize it for %s? [Y/n] ",
			tui.Bold(tui.AbbreviateHome(cwd)), tui.Bold(tool))
		if !tui.ConfirmDefault(prompt, true) {
			return nil
		}
		initArgs := []string{"init"}
		if tool != "" {
			initArgs = append(initArgs, "--tools", tool)
		}
		if err := dispatch(root, initArgs...); err != nil {
			return err
		}
	}
	return dispatch(root, args...)
}

// profileTool returns the tool a "@name" profile launches, or "" if unknown.
func profileTool(at string) string {
	name := strings.TrimPrefix(at, "@")
	glob, err := config.LoadGlobal()
	if err != nil {
		return ""
	}
	return glob.Profiles[name].Tool
}

// runChoice is one launchable target in the Run picker.
type runChoice struct {
	label string   // menu row text
	args  []string // dispatch args, e.g. {"run", "claude"} or {"run", "@work"}
}

// runChoices lists the installed tools and saved profiles you can launch, with
// the workspace's default tool marked. It is split from runMenu so the catalog
// can be tested without a terminal.
func runChoices() []runChoice {
	manifests, err := adapter.Load()
	if err != nil {
		return nil
	}

	var choices []runChoice
	def := defaultTool()

	// Installed tools, default first so the common case is the top row, the rest
	// in detect's stable (alphabetical) order.
	tools := make([]detect.ToolStatus, 0)
	for _, t := range detect.Tools(manifests) {
		if t.Installed {
			tools = append(tools, t)
		}
	}
	sort.SliceStable(tools, func(i, j int) bool {
		return tools[i].Key == def && tools[j].Key != def
	})
	for _, t := range tools {
		hint := t.Name
		if t.Key == def {
			hint += " · default"
		}
		choices = append(choices, runChoice{
			label: fmt.Sprintf("%-9s %s", t.Key, tui.Grey(hint)),
			args:  []string{"run", t.Key},
		})
	}

	if glob, err := config.LoadGlobal(); err == nil && len(glob.Profiles) > 0 {
		names := make([]string, 0, len(glob.Profiles))
		for n := range glob.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			choices = append(choices, runChoice{
				label: fmt.Sprintf("@%-8s %s", n, tui.Grey("profile → "+glob.Profiles[n].Tool)),
				args:  []string{"run", "@" + n},
			})
		}
	}
	return choices
}

// runMenu asks which tool or profile to launch and returns the `run` arguments
// to dispatch. It returns ok=false when the user cancels. With nothing to pick
// from (no detected tools, no profiles) or a single obvious choice, it skips the
// submenu and lets `run` resolve the default.
func runMenu() (args []string, ok bool) {
	choices := runChoices()
	switch len(choices) {
	case 0:
		return []string{"run"}, true // nothing detected; let `run` decide
	case 1:
		return choices[0].args, true // only one option; don't make them pick
	}

	opts := make([]string, len(choices))
	for i, c := range choices {
		opts[i] = c.label
	}
	prompt := tui.Cyan("  ⚓ ") + tui.Bold("Run an agent") + tui.Grey("  · pick a tool or profile")
	if ctx := contextLine(); ctx != "" {
		prompt += "\r\n" + ctx // show where it will launch
	}
	menu := tui.Menu{
		Prompt:    prompt,
		Footer:    tui.Grey("  ↑/↓ move · ⏎ launch · q back"),
		Options:   opts,
		Highlight: tui.HighlightRow,
		TopPad:    true,
	}
	idx, _, err := menu.Run()
	if err != nil || idx < 0 {
		return nil, false
	}
	return choices[idx].args, true
}

// defaultTool reports the current workspace's default tool, best-effort. An
// empty string (not in a workspace, or unreadable config) just means no row is
// marked "default" in the Run picker.
func defaultTool() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	ws, err := workspace.Resolve(cwd)
	if err != nil {
		return ""
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return ""
	}
	return proj.DefaultTool
}

// dispatch runs a subcommand from the menu. We can't call sub.Execute() — cobra
// redirects that back to the root, which (with no os.Args) would just re-open
// this menu. Instead we set the root's args and re-run from the root so cobra
// parses and dispatches for real.
func dispatch(root *cobra.Command, args ...string) error {
	root.SetArgs(args)
	return root.Execute()
}
