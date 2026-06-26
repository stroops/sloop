package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/workspace"
)

func RunStatus(startDir string, w io.Writer) error {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return err
	}
	tool := proj.DefaultTool
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	m := manifests[tool]
	fmt.Fprintf(w, "⚓ %s · %s · agents:%s · ctx:%s · skills:%s\n",
		ws.Name, tool,
		syncpkg.AgentsState(ws.Root),
		syncpkg.ContextState(ws.Root, m),
		syncpkg.SkillsState(ws.Root, ws.SloopDir(), m),
	)
	return nil
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current sloop workspace status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return RunStatus(cwd, cmd.OutOrStdout())
	},
}

func RegisterStatus(cmd *cobra.Command) { cmd.AddCommand(statusCmd) }
