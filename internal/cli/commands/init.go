package commands

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/detect"
	"github.com/stroops/sloop/internal/hints"
	scanpkg "github.com/stroops/sloop/internal/scan"
	"github.com/stroops/sloop/internal/session"
	syncpkg "github.com/stroops/sloop/internal/sync"
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
		enabled = []string{"claude"}
	}
	defaultTool := "claude"
	if !contains(enabled, "claude") {
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
	defer store.Close()
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
		summary, err := RunInit(cwd, initScan)
		if err != nil {
			return err
		}
		cmd.Printf("⚓ Initialized sloop workspace in %s\n", filepath.Join(cwd, config.SloopDirName))
		for _, l := range summary {
			cmd.Printf("  %s\n", l)
		}
		cmd.Println("Next: edit AGENTS.md, then `sloop run`.")
		hints.Show(cmd.OutOrStdout(), "init")
		return nil
	},
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
