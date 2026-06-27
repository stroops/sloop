package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/workspace"
)

// RunSkillNew scaffolds a skill under .sloop/skills and ensures it is delivered
// (symlinked) to enabled tools, returning the file path and the targets it is
// now available in.
func RunSkillNew(startDir, name string) (string, []string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(ws.SloopDir(), "skills", name+".md")
	if _, err := os.Stat(path); err == nil {
		return "", nil, fmt.Errorf("skill %q already exists at %s", name, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", nil, err
	}
	body := fmt.Sprintf("# %s\n\nDescribe the reusable workflow or prompt for %q here.\n", name, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", nil, err
	}
	return path, ensureSkillsLinked(ws), nil
}

// ensureSkillsLinked makes sure each enabled tool's skills symlink exists, so a
// new or imported skill is immediately visible to the tools. Idempotent and
// best-effort (a dir symlink means every skill file is shared at once).
func ensureSkillsLinked(ws *workspace.Workspace) []string {
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return nil
	}
	manifests, err := adapter.Load()
	if err != nil {
		return nil
	}
	var live []string
	for _, tool := range proj.Tools {
		m, ok := manifests[tool]
		if !ok || m.Skills.Target == "" {
			continue
		}
		switch a, _ := syncpkg.SyncSkills(ws.Root, ws.SloopDir(), m); a {
		case syncpkg.ActionLinked, syncpkg.ActionRelinked, syncpkg.ActionCopied, syncpkg.ActionSkipped:
			live = append(live, m.Skills.Target)
		}
	}
	return live
}

var skillCmd = &cobra.Command{
	Use:     "skills",
	Aliases: []string{"skill", "sk"},
	Short:   "Manage reusable skills (shared across every tool)",
}

var skillNewCmd = &cobra.Command{
	Use:     "new <name>",
	Aliases: []string{"n"},
	Short:   "Scaffold a new skill under .sloop/skills and link it into your tools",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path, linked, err := RunSkillNew(cwd, args[0])
		if err != nil {
			return err
		}
		cmd.Printf("created %s\n", path)
		if len(linked) > 0 {
			cmd.Printf("available to: %s\n", strings.Join(linked, ", "))
		} else {
			cmd.Printf("run `sloop sync` to link skills into your tools\n")
		}
		return nil
	},
}

func RegisterSkill(cmd *cobra.Command) {
	skillCmd.AddCommand(skillNewCmd)
	skillAddCmd.Flags().StringVar(&skillAddName, "name", "", "name for the imported skill (default: derived from the URL)")
	skillCmd.AddCommand(skillAddCmd)
	cmd.AddCommand(skillCmd)
}
