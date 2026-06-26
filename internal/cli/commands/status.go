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
	prof, err := resolveProfile(ws.SloopDir(), tool, proj.DefaultTool)
	if err != nil {
		return err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	sync := "unknown"
	if m, ok := manifests[prof.Tool]; ok {
		stale, err := syncpkg.Stale(ws.Root, ws.SloopDir(), m, prof)
		if err != nil {
			return err
		}
		if stale {
			sync = "stale"
		} else {
			sync = "fresh"
		}
	}
	fmt.Fprintf(w, "⚓ %s · %s · sync:%s\n", ws.Name, tool, sync)
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
