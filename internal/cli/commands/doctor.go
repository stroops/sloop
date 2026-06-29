package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/detect"
	"github.com/stroops/sloop/internal/tui"
)

func RunDoctor(w io.Writer) error {
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}

	// AI provider CLIs (the tools sloop launches). A missing one isn't an error:
	// you only use some, so it's marked in grey, not alarming red.
	_, _ = fmt.Fprintln(w, "AI provider CLIs:")
	for _, s := range detect.Tools(manifests) {
		if s.Installed {
			ver := ""
			if s.Version != "" {
				ver = "  " + tui.Grey(s.Version)
			}
			_, _ = fmt.Fprintf(w, "  %s %s%s\n", tui.Green("✓"), s.Key, ver)
		} else {
			_, _ = fmt.Fprintf(w, "  %s %s %s\n", tui.Grey("✗"), s.Key, tui.Grey("(not installed)"))
		}
	}

	// Multiplexer is its own group: it powers ps/run/attach, but is optional.
	_, _ = fmt.Fprintln(w, "\nMultiplexer (powers ps / run / attach):")
	if tmux := detect.Tmux(); tmux.Installed {
		_, _ = fmt.Fprintf(w, "  %s tmux %s\n", tui.Green("✓"), tui.Grey(tmux.Version))
	} else {
		_, _ = fmt.Fprintf(w, "  %s %s\n", tui.Grey("✗"), tui.Grey("tmux not found (optional); sloop falls back to plain exec"))
	}

	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	desc := "asks before actions"
	if g.Mode == config.ModeAuto {
		desc = "runs without prompts (assume yes)"
	}
	_, _ = fmt.Fprintf(w, "\nMode: %s %s\n", g.Mode, tui.Grey("· "+desc))
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
