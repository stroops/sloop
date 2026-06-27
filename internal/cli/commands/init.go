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

// RunInit scaffolds the workspace and delivers context for every enabled tool,
// returning a human-readable summary of what it created/linked.
func RunInit(dir string, scan bool) ([]string, error) {
	sloopDir := filepath.Join(dir, config.SloopDirName)
	for _, sub := range []string{"skills", "vault"} {
		if err := os.MkdirAll(filepath.Join(sloopDir, sub), 0o700); err != nil {
			return nil, err
		}
	}

	// Detect installed known tools; always ensure claude is usable.
	manifests, err := adapter.Load()
	if err != nil {
		return nil, err
	}
	enabled := detect.InstalledKeys(manifests)
	if len(enabled) == 0 {
		enabled = []string{fallbackTool}
	}
	defaultTool := fallbackTool
	if !contains(enabled, fallbackTool) {
		defaultTool = enabled[0]
	}

	if err := config.SaveProject(sloopDir, &config.Project{
		Tools:       enabled,
		DefaultTool: defaultTool,
	}); err != nil {
		return nil, err
	}

	if scan {
		if _, err := syncpkg.EnsureAgentsContent(dir, scanpkg.Scan(dir).AgentsMarkdown()); err != nil {
			return nil, err
		}
	} else {
		if _, err := syncpkg.EnsureAgents(dir); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(sloopDir, ".gitignore"),
		[]byte(sloopGitignore), 0o600); err != nil {
		return nil, err
	}

	// Deliver native context (pointer files like CLAUDE.md + skills links) for
	// every enabled tool, so the workspace is usable right after init — not only
	// after the first `sloop sync`/`run`. Best-effort per tool.
	var summary []string
	for _, tool := range enabled {
		m, ok := manifests[tool]
		if !ok {
			continue
		}
		log, err := syncOne(dir, sloopDir, m, false)
		if err != nil {
			return nil, err
		}
		for _, l := range log {
			summary = append(summary, tool+": "+l)
		}
		if initScaffold {
			for _, d := range m.Scaffold {
				target := filepath.Join(dir, d)
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
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a .sloop workspace in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		// Interactive onboarding: when on a real terminal and not under
		// --auto/--no-input, walk a new user through the choices. Otherwise keep
		// the scriptable behavior (flags only).
		wantHooks := false
		if interactiveInit(cmd) {
			r := bufio.NewReader(os.Stdin)
			ix := initInteraction(cmd)
			manifests, _ := adapter.Load()
			det := detect.InstalledKeys(manifests)
			if len(det) == 0 {
				det = []string{fallbackTool}
			}
			cmd.Printf("Detected tools: %s\n", strings.Join(det, ", "))
			cmd.Println(tui.Grey("(edit .sloop/config.yaml later to change which are enabled)"))
			initScan = ix.Ask("Pre-fill AGENTS.md from your codebase?", true, r, cmd.OutOrStdout())
			initScaffold = ix.Ask("Create each tool's standard folders?", false, r, cmd.OutOrStdout())
			wantHooks = ix.Ask("Install status hooks for precise `sloop ps`?", true, r, cmd.OutOrStdout())
		}

		summary, err := RunInit(cwd, initScan)
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
		cmd.Println("Next: edit AGENTS.md, then `sloop run`.")
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
	initCmd.Flags().BoolVarP(&initScaffold, "scaffold", "S", false, "also create each enabled tool's standard folders (.claude/skills, .cursor/rules, …)")
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
