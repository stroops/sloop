package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/hints"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/tui"
	"github.com/stroops/sloop/internal/workspace"
)

// checkItem is one readiness line: whether it passed, a label, and the command
// that fixes it when it hasn't. Info items are advisory (not counted as a gap).
type checkItem struct {
	OK    bool
	Info  bool
	Label string
	Fix   string
}

// readinessChecklist builds the workspace readiness checklist. Criteria are NOT
// sloop's own opinion: the per-tool checks are derived from each enabled tool's
// adapter manifest (context/skills/hooks — the single provider-aware source) and
// the on-disk delivery state. Only workspace-level basics (AGENTS.md, a default
// tool) are sloop's own minimal model. Pure aside from filesystem reads, so it's
// directly testable.
func readinessChecklist(root, sloopDir string, proj *config.Project, manifests map[string]adapter.Manifest) []checkItem {
	var items []checkItem

	// Workspace-level basics.
	items = append(items, checkItem{
		OK:    syncpkg.AgentsState(root) == "ok",
		Label: "AGENTS.md present (canonical context)",
		Fix:   "sloop init",
	})
	_, hasDefault := manifests[proj.DefaultTool]
	items = append(items, checkItem{
		OK:    proj.DefaultTool != "" && hasDefault,
		Label: "Default tool set in .sloop/config.yaml",
		Fix:   "set default_tool in .sloop/config.yaml",
	})

	// Per enabled tool, sourced from its manifest.
	hasSkills := skillCount(sloopDir) > 0
	for _, t := range proj.Tools {
		m, ok := manifests[t]
		if !ok {
			continue
		}

		// Context delivery (pointer-mode tools only; native tools need nothing).
		switch syncpkg.ContextState(root, m) {
		case "ok":
			items = append(items, checkItem{OK: true, Label: m.Name + ": context delivered (" + m.Context.File + ")"})
		case "foreign":
			items = append(items, checkItem{Label: m.Name + ": " + m.Context.File + " is a hand-authored file, not a sloop pointer", Fix: "sloop sync --repair"})
		case "missing":
			items = append(items, checkItem{Label: m.Name + ": context pointer not delivered (" + m.Context.File + ")", Fix: "sloop sync"})
		}

		// Skills linked — only relevant once there are skills to deliver.
		if m.Skills.Target != "" && hasSkills {
			if syncpkg.SkillsState(root, sloopDir, m) == "linked" {
				items = append(items, checkItem{OK: true, Label: m.Name + ": skills linked"})
			} else {
				items = append(items, checkItem{Label: m.Name + ": skills not linked into " + m.Skills.Target, Fix: "sloop sync"})
			}
		}

		// Status hooks — only flag tools sloop can auto-install for.
		if hookInstaller(m.Hooks.Install) != nil {
			if hooksInstalledFor(root, m) {
				items = append(items, checkItem{OK: true, Label: m.Name + ": status hooks installed"})
			} else {
				items = append(items, checkItem{Label: m.Name + ": status hooks not installed", Fix: "sloop hooks install " + t})
			}
		}
	}

	return items
}

// renderChecklist writes the checklist with ✓/✗ marks, a fix command per gap,
// and a one-line summary — a checklist of best practices, deliberately not a
// score (sloop checks presence, it can't judge content quality).
func renderChecklist(w io.Writer, name string, items []checkItem) {
	_, _ = fmt.Fprintf(w, "⚓ Workspace readiness — %s\n\n", tui.Bold(name))
	gaps := 0
	for _, it := range items {
		switch {
		case it.OK:
			_, _ = fmt.Fprintf(w, "  %s %s\n", tui.Green("✓"), it.Label)
		case it.Info:
			_, _ = fmt.Fprintf(w, "  %s %s\n", tui.Grey("•"), it.Label)
		default:
			gaps++
			line := fmt.Sprintf("  %s %s", tui.Yellow("✗"), it.Label)
			if it.Fix != "" {
				line += tui.Grey("   → " + it.Fix)
			}
			_, _ = fmt.Fprintln(w, line)
		}
	}
	if gaps == 0 {
		_, _ = fmt.Fprintf(w, "\n%s\n", tui.Green("Ready — every best practice is in place."))
		return
	}
	_, _ = fmt.Fprintf(w, "\n%s\n", tui.Yellow(fmt.Sprintf("%d to improve", gaps))+tui.Grey(" — run the suggested commands above."))
}

// RunCheck resolves the workspace and prints its readiness checklist.
func RunCheck(startDir string, w io.Writer) error {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	renderChecklist(w, ws.Name, readinessChecklist(ws.Root, ws.SloopDir(), proj, manifests))
	return nil
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check this workspace against AI-readiness best practices (a checklist, not a score)",
	Long: `Run a best-practices checklist for the current workspace: is AGENTS.md present,
is context delivered to each enabled tool, are skills linked, are status hooks
installed? Each gap comes with the command that fixes it.

The per-tool criteria come from each tool's adapter manifest (context/skills/
hooks), not from sloop's own opinion — so they deepen as the manifests do.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := RunCheck(cwd, cmd.OutOrStdout()); err != nil {
			return err
		}
		hints.Show(cmd.OutOrStdout(), "check")
		return nil
	},
}

func RegisterCheck(cmd *cobra.Command) { cmd.AddCommand(checkCmd) }
