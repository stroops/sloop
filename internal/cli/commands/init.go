package commands

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/detect"
	"github.com/stroops/sloop/internal/hints"
	scanpkg "github.com/stroops/sloop/internal/scan"
	"github.com/stroops/sloop/internal/session"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/tui"
)

const sloopGitignore = `# Local, machine-specific caches
cache/
*.local
# Personal notes — not shared with the team
vault/
`

// defaultPrimary is the single tool a bare `init` enables — the user's installed
// default (claude when present), keeping a fresh workspace minimal rather than
// turning on every CLI on the machine.
func defaultPrimary(manifests map[string]adapter.Manifest) string {
	detected := detect.InstalledKeys(manifests)
	if len(detected) == 0 || contains(detected, fallbackTool) {
		return fallbackTool
	}
	return detected[0]
}

// RunInit scaffolds the workspace and enables the given tools (nil = a minimal
// single-tool default), delivering context only when the workspace actually
// needs sloop's portable layer (see needsCanonical). Returns a summary of what
// it created/linked.
func RunInit(dir string, tools []string, scan bool) ([]string, error) {
	sloopDir := filepath.Join(dir, config.SloopDirName)
	for _, sub := range []string{"skills", "vault"} {
		if err := os.MkdirAll(filepath.Join(sloopDir, sub), 0o700); err != nil {
			return nil, err
		}
	}

	manifests, err := adapter.Load()
	if err != nil {
		return nil, err
	}
	if len(tools) == 0 {
		tools = []string{defaultPrimary(manifests)}
	}
	defaultTool := tools[0]
	if contains(tools, fallbackTool) {
		defaultTool = fallbackTool
	}

	if err := config.SaveProject(sloopDir, &config.Project{
		Tools:       tools,
		DefaultTool: defaultTool,
	}); err != nil {
		return nil, err
	}

	// The portable-context layer (AGENTS.md + pointers) is only set up when the
	// workspace needs it; a lone pointer-mode tool keeps its own context file.
	canonical := needsCanonical(tools, manifests)
	if canonical && scan {
		if _, err := syncpkg.EnsureAgentsContent(dir, scanpkg.Scan(dir).AgentsMarkdown()); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(sloopDir, ".gitignore"),
		[]byte(sloopGitignore), 0o600); err != nil {
		return nil, err
	}

	// Deliver context (when canonical) + skills links for every enabled tool, so
	// the workspace is usable right after init. Best-effort per tool.
	var summary []string
	for _, tool := range tools {
		m, ok := manifests[tool]
		if !ok {
			continue
		}
		log, err := syncOne(dir, sloopDir, m, false, canonical)
		if err != nil {
			return nil, err
		}
		for _, l := range log {
			summary = append(summary, tool+": "+l)
		}
		if initScaffold {
			for _, d := range m.Scaffold {
				target := filepath.Join(dir, d)
				if _, err := os.Stat(target); err == nil {
					continue // already there — don't claim to have scaffolded it
				}
				if err := os.MkdirAll(target, 0o755); err != nil {
					return nil, err
				}
				_ = os.WriteFile(filepath.Join(target, ".gitkeep"), nil, 0o644)
				summary = append(summary, "scaffolded "+d)
			}
		}
	}

	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return summary, err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return summary, err
	}
	defer func() { _ = store.Close() }()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return summary, err
	}
	if _, err := store.RegisterWorkspace(filepath.Base(abs), abs); err != nil {
		return summary, err
	}
	return summary, nil
}

var (
	initScan     bool
	initScaffold bool
	initTools    string
)

// resolveInitTools decides which tools `init` enables: the explicit --tools list
// if given, else an interactive per-tool prompt (every detected tool is asked, the
// primary defaulted on), else a minimal single-tool default. r==nil means
// non-interactive. Keeps a fresh workspace from turning on every CLI installed.
func resolveInitTools(cmd *cobra.Command, manifests map[string]adapter.Manifest, r *bufio.Reader, ix Interaction) []string {
	if initTools != "" {
		var out []string
		for _, t := range strings.Split(initTools, ",") {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if _, has := manifests[t]; !has {
				cmd.Printf("%s\n", tui.Grey("ignoring unknown tool: "+t))
				continue
			}
			if !contains(out, t) {
				out = append(out, t)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	detected := detect.InstalledKeys(manifests)
	if len(detected) == 0 {
		detected = []string{fallbackTool}
	}
	primary := defaultPrimary(manifests)

	if r == nil {
		return []string{primary} // minimal default
	}

	// Ask about every detected tool (including the primary — it isn't always
	// Claude), primary first and defaulted on so a quick Enter keeps it simple.
	cmd.Printf("Detected tools: %s\n", strings.Join(detected, ", "))
	var selected []string
	for _, t := range primaryFirst(detected, primary) {
		if ix.Ask("Use "+displayTool(t, manifests)+" in this workspace?", t == primary, r, cmd.OutOrStdout()) {
			selected = append(selected, t)
		}
	}
	if len(selected) == 0 {
		cmd.Println(tui.Grey("none selected — enabling " + displayTool(primary, manifests)))
		selected = []string{primary}
	}
	return selected
}

// primaryFirst returns detected tools with the primary moved to the front, so the
// most-likely tool (e.g. Claude) is the first one init asks about.
func primaryFirst(detected []string, primary string) []string {
	out := make([]string, 0, len(detected))
	if contains(detected, primary) {
		out = append(out, primary)
	}
	for _, t := range detected {
		if t != primary {
			out = append(out, t)
		}
	}
	return out
}

// hooksNeeded reports whether any enabled tool's status hooks aren't installed.
func hooksNeeded(root string, tools []string, manifests map[string]adapter.Manifest) bool {
	for _, t := range tools {
		m := manifests[t]
		if hookInstaller(m.Hooks.Install) != nil && !hooksInstalledFor(root, m) {
			return true
		}
	}
	return false
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a .sloop workspace in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		manifests, _ := adapter.Load()

		// One reader for the whole interactive flow (two readers on stdin would
		// race over buffered input).
		var r *bufio.Reader
		var ix Interaction
		if interactiveInit(cmd) {
			r = bufio.NewReader(os.Stdin)
			ix = initInteraction(cmd)
		}
		tools := resolveInitTools(cmd, manifests, r, ix)
		canonical := needsCanonical(tools, manifests)

		// Only ask about work that isn't already done.
		wantHooks := false
		if r != nil {
			if canonical && syncpkg.AgentsState(cwd) != "ok" {
				initScan = ix.Ask("Pre-fill AGENTS.md from your codebase?", true, r, cmd.OutOrStdout())
			}
			// Scaffolding provider folders isn't asked: tools create their own
			// layout, and sloop already creates the skills dir it needs. It stays
			// available as the explicit `--scaffold` opt-in.
			if hooksNeeded(cwd, tools, manifests) {
				wantHooks = ix.Ask("Install status hooks for precise `sloop ps`?", true, r, cmd.OutOrStdout())
			}
		}

		summary, err := RunInit(cwd, tools, initScan)
		if err != nil {
			return err
		}
		cmd.Printf("⚓ Initialized sloop workspace in %s\n", filepath.Join(cwd, config.SloopDirName))
		for _, l := range summary {
			cmd.Printf("  %s\n", l)
		}
		if wantHooks {
			for _, l := range installHooksForEnabled(cwd) {
				cmd.Printf("  %s\n", l)
			}
		}
		if canonical {
			cmd.Println("Next: edit AGENTS.md, then `sloop run`.")
		} else {
			cmd.Printf("Next: %s keeps its own context (e.g. run it and `/init`). sloop manages the fleet & skills here.\n",
				displayTool(tools[0], manifests))
		}
		hints.Show(cmd.OutOrStdout(), "init")
		return nil
	},
}

// initInteraction resolves auto/no-input from the global flags + config mode.
func initInteraction(cmd *cobra.Command) Interaction {
	autoFlag, _ := cmd.Flags().GetBool("auto")
	noInput, _ := cmd.Flags().GetBool("no-input")
	gmode := ""
	if g, err := config.LoadGlobal(); err == nil {
		gmode = g.Mode
	}
	return ResolveInteraction("", gmode, autoFlag, noInput)
}

// interactiveInit reports whether init should prompt: a real terminal and not
// forced non-interactive.
func interactiveInit(cmd *cobra.Command) bool {
	ix := initInteraction(cmd)
	return !ix.Auto && !ix.NoInput && term.IsTerminal(int(os.Stdin.Fd()))
}

// installHooksForEnabled installs status hooks for each enabled settings-json
// tool (claude/gemini), reusing the hooks installer. Best-effort.
func installHooksForEnabled(root string) []string {
	var log []string
	proj, err := config.LoadProject(filepath.Join(root, config.SloopDirName))
	if err != nil {
		return log
	}
	manifests, err := adapter.Load()
	if err != nil {
		return log
	}
	for _, tool := range proj.Tools {
		m, ok := manifests[tool]
		if !ok || m.Hooks.Install != "settings-json" {
			continue
		}
		path, err := resolveHookConfigPath(root, m.Hooks.Config)
		if err != nil {
			continue
		}
		if changed, err := installSettingsHooks(path, eventCommands(m.Hooks)); err == nil && changed {
			log = append(log, "hooks: installed for "+tool)
		}
	}
	return log
}

func RegisterInit(cmd *cobra.Command) {
	initCmd.Flags().BoolVarP(&initScan, "scan", "s", false, "scan the existing codebase to pre-fill AGENTS.md")
	initCmd.Flags().BoolVarP(&initScaffold, "scaffold", "S", false, "opt-in: also create each enabled tool's standard folders (.claude/skills, .cursor/rules, …); off by default since tools create their own")
	initCmd.Flags().StringVar(&initTools, "tools", "", "comma-separated tools to enable for this workspace (default: your primary tool)")
	cmd.AddCommand(initCmd)
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
