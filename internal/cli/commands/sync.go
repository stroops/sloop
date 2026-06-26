package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/profile"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/workspace"
)

func resolveProfile(sloopDir, target, defaultTool string) (profile.Profile, error) {
	if target == "" {
		target = defaultTool
	}
	if target == "" {
		return profile.Profile{}, fmt.Errorf("no target tool or profile given and no default_tool set")
	}
	// A profile file wins over a bare tool name.
	profPath := filepath.Join(sloopDir, "profiles", target+".yaml")
	if _, err := os.Stat(profPath); err == nil {
		return profile.Load(profPath)
	}
	return profile.Default(target), nil
}

// RunSync resolves the workspace + profile and synchronizes v2 context.
func RunSync(startDir, target string) ([]string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return nil, err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return nil, err
	}
	prof, err := resolveProfile(ws.SloopDir(), target, proj.DefaultTool)
	if err != nil {
		return nil, err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return nil, err
	}
	m, ok := manifests[prof.Tool]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q (no adapter)", prof.Tool)
	}

	var log []string
	if a, err := syncpkg.EnsureAgents(ws.Root); err != nil {
		return nil, err
	} else if a == syncpkg.ActionCreated {
		log = append(log, "created AGENTS.md")
	}
	switch a, err := syncpkg.SyncContext(ws.Root, m); {
	case err != nil:
		return nil, err
	case a == syncpkg.ActionCreated:
		log = append(log, "created "+m.Context.File)
	case a == syncpkg.ActionForeign:
		log = append(log, m.Context.File+" exists, left as-is")
	}
	switch a, err := syncpkg.SyncSkills(ws.Root, ws.SloopDir(), m); {
	case err != nil:
		return nil, err
	case a == syncpkg.ActionLinked:
		log = append(log, "linked "+m.Skills.Target)
	case a == syncpkg.ActionCopied:
		log = append(log, "copied skills to "+m.Skills.Target)
	case a == syncpkg.ActionForeign:
		log = append(log, m.Skills.Target+" exists, left as-is")
	}
	return log, nil
}

var syncWorkspace string

var syncCmd = &cobra.Command{
	Use:   "sync [tool|profile]",
	Short: "Regenerate native context files from .sloop",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		startDir, err := resolveStartDir(cwd, syncWorkspace)
		if err != nil {
			return err
		}
		target := ""
		if len(args) == 1 {
			target = args[0]
		}
		written, err := RunSync(startDir, target)
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
	cmd.AddCommand(syncCmd)
}
