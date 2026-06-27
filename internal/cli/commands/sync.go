package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/workspace"
)

// resolveTool picks the target tool: the given target, or the project default.
// (Enabled tools live in .sloop/config.yaml; there is no per-tool profile file.)
func resolveTool(target, defaultTool string) (string, error) {
	if target == "" {
		target = defaultTool
	}
	if target == "" {
		return "", fmt.Errorf("no target tool given and no default_tool set")
	}
	return target, nil
}

// RunSync resolves the workspace + tool and synchronizes v2 context.
func RunSync(startDir, target string, repair bool) ([]string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return nil, err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return nil, err
	}
	tool, err := resolveTool(target, proj.DefaultTool)
	if err != nil {
		return nil, err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return nil, err
	}
	m, ok := manifests[tool]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q (no adapter)", tool)
	}

	return syncOne(ws.Root, ws.SloopDir(), m, repair)
}

func syncOne(root, sloopDir string, m adapter.Manifest, repair bool) ([]string, error) {
	var log []string
	if a, err := syncpkg.EnsureAgents(root); err != nil {
		return nil, err
	} else if a == syncpkg.ActionCreated {
		log = append(log, "created AGENTS.md")
	}
	ctx := syncpkg.SyncContext
	skl := syncpkg.SyncSkills
	if repair {
		ctx = syncpkg.RepairContext
		skl = func(r, s string, mm adapter.Manifest) (syncpkg.Action, error) { return syncpkg.RepairSkills(r, s, mm) }
	}

	switch a, err := ctx(root, m); {
	case err != nil:
		return nil, err
	case a == syncpkg.ActionCreated:
		log = append(log, "created "+m.Context.File)
	case a == syncpkg.ActionForeign:
		log = append(log, m.Context.File+" exists, left as-is")
	case a == syncpkg.ActionRepaired:
		log = append(log, "repaired "+m.Context.File)
	}
	switch a, err := skl(root, sloopDir, m); {
	case err != nil:
		return nil, err
	case a == syncpkg.ActionLinked:
		log = append(log, "linked "+m.Skills.Target)
	case a == syncpkg.ActionRelinked:
		log = append(log, "relinked "+m.Skills.Target)
	case a == syncpkg.ActionCopied:
		log = append(log, "copied skills to "+m.Skills.Target)
	case a == syncpkg.ActionBroken:
		log = append(log, "skills source .sloop/skills missing (left "+m.Skills.Target+")")
	case a == syncpkg.ActionForeign:
		log = append(log, m.Skills.Target+" exists, left as-is")
	case a == syncpkg.ActionRepaired:
		log = append(log, "repaired "+m.Skills.Target)
	}
	return log, nil
}

func RunSyncAll(startDir string, repair bool) ([]string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return nil, err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return nil, err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return nil, err
	}
	var log []string
	for _, tool := range proj.Tools {
		m, ok := manifests[tool]
		if !ok {
			log = append(log, tool+": unknown tool (no adapter), skipped")
			continue
		}
		lines, err := syncOne(ws.Root, ws.SloopDir(), m, repair)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tool, err)
		}
		for _, l := range lines {
			log = append(log, tool+": "+l)
		}
	}
	return log, nil
}

var syncWorkspace string
var syncAll bool
var syncRepair bool

var syncCmd = &cobra.Command{
	Use:     "sync [tool|profile]",
	Aliases: []string{"s"},
	Short:   "Regenerate native context files from .sloop",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		startDir, err := resolveStartDir(cwd, syncWorkspace)
		if err != nil {
			return err
		}
		if syncAll {
			if len(args) > 0 {
				return fmt.Errorf("--all takes no tool argument")
			}
			written, err := RunSyncAll(startDir, syncRepair)
			if err != nil {
				return err
			}
			for _, w := range written {
				cmd.Printf("synced %s\n", w)
			}
			return nil
		}
		target := ""
		if len(args) == 1 {
			target = args[0]
		}
		written, err := RunSync(startDir, target, syncRepair)
		if err != nil {
			return err
		}
		for _, w := range written {
			cmd.Printf("synced %s\n", w)
		}
		return nil
	},
}

func RegisterSync(cmd *cobra.Command) {
	syncCmd.Flags().StringVarP(&syncWorkspace, "workspace", "w", "", "target a registered workspace by name")
	syncCmd.Flags().BoolVarP(&syncAll, "all", "a", false, "sync every enabled tool")
	syncCmd.Flags().BoolVarP(&syncRepair, "repair", "r", false, "safely repair broken skills links or foreign context files")
	syncCmd.ValidArgsFunction = completeTools
	_ = syncCmd.RegisterFlagCompletionFunc("workspace", completeWorkspaces)
	cmd.AddCommand(syncCmd)
}
