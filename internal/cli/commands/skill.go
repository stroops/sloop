package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/workspace"
)

func RunSkillNew(startDir, name string) (string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(ws.SloopDir(), "skills", name+".md")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("skill %q already exists at %s", name, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body := fmt.Sprintf("# %s\n\nDescribe the reusable workflow or prompt for %q here.\n", name, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage reusable skills",
}

var skillNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Scaffold a new skill file under .sloop/skills",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path, err := RunSkillNew(cwd, args[0])
		if err != nil {
			return err
		}
		cmd.Printf("created %s\n", path)
		return nil
	},
}

func RegisterSkill(cmd *cobra.Command) {
	skillCmd.AddCommand(skillNewCmd)
	cmd.AddCommand(skillCmd)
}
