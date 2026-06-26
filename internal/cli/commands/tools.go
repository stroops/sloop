package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/detect"
)

func RunTools(w io.Writer) error {
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	for _, s := range detect.Tools(manifests) {
		status := "missing"
		if s.Installed {
			status = "installed"
			if s.Version != "" {
				status += " (" + s.Version + ")"
			}
		}
		fmt.Fprintf(w, "%-10s %-14s %s\n", s.Key, s.Name, status)
	}
	return nil
}

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List configured AI tool adapters and their install status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunTools(cmd.OutOrStdout())
	},
}

func RegisterTools(cmd *cobra.Command) { cmd.AddCommand(toolsCmd) }
