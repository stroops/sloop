package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/detect"
)

func RunDoctor(w io.Writer) error {
	_, _ = fmt.Fprintln(w, "Tools:")
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	for _, s := range detect.Tools(manifests) {
		mark := "✗"
		extra := ""
		if s.Installed {
			mark = "✓"
			if s.Version != "" {
				extra = " " + s.Version
			}
		}
		_, _ = fmt.Fprintf(w, "  %s %s%s\n", mark, s.Key, extra)
	}

	tmux := detect.Tmux()
	mark := "✗ (optional — exec fallback in use)"
	if tmux.Installed {
		mark = "✓ " + tmux.Version
	}
	_, _ = fmt.Fprintf(w, "tmux: %s\n", mark)

	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "mode: %s\n", g.Mode)
	return nil
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the sloop environment (tools, tmux, config)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunDoctor(cmd.OutOrStdout())
	},
}

func RegisterDoctor(cmd *cobra.Command) { cmd.AddCommand(doctorCmd) }
