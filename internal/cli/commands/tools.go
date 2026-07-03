package commands

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/detect"
	"github.com/stroops/sloop/internal/hints"
)

func RunTools(w io.Writer) error {
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tNAME\tINSTALLED\tCONTEXT\tSKILLS\tHOOKS")
	for _, s := range detect.Tools(manifests) {
		status := "missing"
		if s.Installed {
			status = "installed"
			if v := shortVersion(s.Version); v != "" {
				status += " " + v
			}
		}
		m := manifests[s.Key]
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Key, s.Name, status, contextLabel(m), skillsLabel(m), hooksLabel(m))
	}
	return tw.Flush()
}

// shortVersion keeps just the leading version token (e.g. "2.1.195" from
// "2.1.195 (Claude Code)") so the matrix columns stay aligned.
func shortVersion(v string) string {
	if i := strings.IndexAny(v, " \t"); i >= 0 {
		return v[:i]
	}
	return v
}

// contextLabel/skillsLabel/hooksLabel summarize a manifest's per-provider
// capabilities for the matrix, the runtime view of where sloop is
// provider-aware (all read from the adapter manifest, the single source).
func contextLabel(m adapter.Manifest) string {
	if m.Context.Mode == "" {
		return "-"
	}
	return m.Context.Mode
}

func skillsLabel(m adapter.Manifest) string {
	if m.Skills.Target != "" {
		return "yes"
	}
	return "no"
}

func hooksLabel(m adapter.Manifest) string {
	if hookInstaller(m.Hooks.Install) != nil {
		return "auto"
	}
	if m.Hooks.Events.Working.Event != "" || m.Hooks.Events.Waiting.Event != "" || m.Hooks.Events.Idle.Event != "" {
		return "manual"
	}
	return "-"
}

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List configured AI tool adapters and their install status",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RunTools(cmd.OutOrStdout()); err != nil {
			return err
		}
		hints.Show(cmd.OutOrStdout(), "tools")
		return nil
	},
}

func RegisterTools(cmd *cobra.Command) { cmd.AddCommand(toolsCmd) }
