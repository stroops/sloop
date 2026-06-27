package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/detect"
)

func RunTools(w io.Writer) error {
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%-10s %-16s %-16s %-9s %-7s %s\n",
		"KEY", "NAME", "INSTALLED", "CONTEXT", "SKILLS", "HOOKS")
	for _, s := range detect.Tools(manifests) {
		status := "missing"
		if s.Installed {
			status = "installed"
			if v := shortVersion(s.Version); v != "" {
				status += " " + v
			}
		}
		m := manifests[s.Key]
		fmt.Fprintf(w, "%-10s %-16s %-16s %-9s %-7s %s\n",
			s.Key, s.Name, status, contextLabel(m), skillsLabel(m), hooksLabel(m))
	}
	return nil
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
// capabilities for the matrix — the runtime view of where sloop is
// provider-aware (all read from the adapter manifest, the single source).
func contextLabel(m adapter.Manifest) string {
	if m.Context.Mode == "" {
		return "—"
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
	if m.Hooks.Install == "settings-json" {
		return "auto"
	}
	if m.Hooks.Events.Working != "" || m.Hooks.Events.Waiting != "" || m.Hooks.Events.Idle != "" {
		return "manual"
	}
	return "—"
}

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List configured AI tool adapters and their install status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunTools(cmd.OutOrStdout())
	},
}

func RegisterTools(cmd *cobra.Command) { cmd.AddCommand(toolsCmd) }
