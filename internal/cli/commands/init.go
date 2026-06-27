package commands

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/detect"
	"github.com/stroops/sloop/internal/profile"
	scanpkg "github.com/stroops/sloop/internal/scan"
	"github.com/stroops/sloop/internal/session"
	syncpkg "github.com/stroops/sloop/internal/sync"
)

const sloopGitignore = `# Local, machine-specific caches
cache/
*.local
`

func RunInit(dir string, scan bool) error {
	sloopDir := filepath.Join(dir, config.SloopDirName)
	for _, sub := range []string{"skills", "vault", "profiles"} {
		if err := os.MkdirAll(filepath.Join(sloopDir, sub), 0o755); err != nil {
			return err
		}
	}

	// Detect installed known tools; always ensure claude is usable.
	manifests, err := adapter.Load()
	if err != nil {
		return err
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
		return err
	}

	if scan {
		if _, err := syncpkg.EnsureAgentsContent(dir, scanpkg.Scan(dir).AgentsMarkdown()); err != nil {
			return err
		}
	} else {
		if _, err := syncpkg.EnsureAgents(dir); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(sloopDir, ".gitignore"),
		[]byte(sloopGitignore), 0o644); err != nil {
		return err
	}
	for _, tool := range enabled {
		if err := profile.Save(filepath.Join(sloopDir, "profiles", tool+".yaml"),
			profile.Default(tool)); err != nil {
			return err
		}
	}

	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	_, err = store.RegisterWorkspace(filepath.Base(abs), abs)
	return err
}

var initScan bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a .sloop workspace in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := RunInit(cwd, initScan); err != nil {
			return err
		}
		cmd.Printf("⚓ Initialized sloop workspace in %s\n", filepath.Join(cwd, config.SloopDirName))
		return nil
	},
}

func RegisterInit(cmd *cobra.Command) {
	initCmd.Flags().BoolVarP(&initScan, "scan", "s", false, "scan the existing codebase to pre-fill AGENTS.md")
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
