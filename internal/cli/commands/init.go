package commands

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/profile"
	"github.com/stroops/sloop/internal/session"
)

const starterContext = `# Project Context

Describe this project so AI tools start with the right background.
`

const sloopGitignore = `# Local, machine-specific caches
cache/
*.local
`

func RunInit(dir string) error {
	sloopDir := filepath.Join(dir, config.SloopDirName)
	for _, sub := range []string{"context", "skills", "vault", "profiles"} {
		if err := os.MkdirAll(filepath.Join(sloopDir, sub), 0o755); err != nil {
			return err
		}
	}

	if err := config.SaveProject(sloopDir, &config.Project{
		Tools:       []string{"claude"},
		DefaultTool: "claude",
	}); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(sloopDir, "context", "project.md"),
		[]byte(starterContext), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sloopDir, ".gitignore"),
		[]byte(sloopGitignore), 0o644); err != nil {
		return err
	}
	if err := profile.Save(filepath.Join(sloopDir, "profiles", "claude.yaml"),
		profile.Default("claude")); err != nil {
		return err
	}

	// Register the workspace in the global DB (best-effort).
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

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a .sloop workspace in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := RunInit(cwd); err != nil {
			return err
		}
		cmd.Printf("⚓ Initialized sloop workspace in %s\n", filepath.Join(cwd, config.SloopDirName))
		return nil
	},
}

func RegisterInit(cmd *cobra.Command) { cmd.AddCommand(initCmd) }
