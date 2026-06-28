package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/hints"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/tui"
	"github.com/stroops/sloop/internal/workspace"
)

// git helpers are seams (overridable in tests) so the checklist can verify a
// file is committed without a real repo in the test.
var (
	inGitRepo = func(root string) bool {
		return exec.Command("git", "-C", root, "rev-parse", "--is-inside-work-tree").Run() == nil
	}
	gitTracked = func(root, path string) bool {
		return exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", path).Run() == nil
	}
)

// evalReadiness evaluates one declarative manifest check against the filesystem.
func evalReadiness(root string, c adapter.ReadinessCheck) bool {
	p := filepath.Join(root, c.Path)
	switch c.Kind {
	case "file-exists":
		fi, err := os.Stat(p)
		return err == nil && !fi.IsDir()
	case "dir-exists":
		fi, err := os.Stat(p)
		return err == nil && fi.IsDir()
	case "git-tracked":
		return inGitRepo(root) && gitTracked(root, c.Path)
	}
	return false
}

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
	agentsOK := syncpkg.AgentsState(root) == "ok"
	items = append(items, checkItem{
		OK:    agentsOK,
		Label: "AGENTS.md present (canonical context)",
		Fix:   "sloop init",
	})
	// Canonical context should be committed so the team shares it (a best practice
	// every provider echoes). Only meaningful once it exists and we're in a repo.
	if agentsOK && inGitRepo(root) {
		items = append(items, checkItem{
			OK:    gitTracked(root, "AGENTS.md"),
			Label: "AGENTS.md committed to git (shared with your team)",
			Fix:   "git add AGENTS.md && git commit",
		})
	}
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

		// Extra best practices declared in the manifest (sourced from the provider's
		// own docs). Optional ones are advisory so power-user features don't nag.
		for _, rc := range m.Readiness.Checks {
			switch {
			case evalReadiness(root, rc):
				items = append(items, checkItem{OK: true, Label: m.Name + ": " + rc.Label})
			case rc.Optional:
				items = append(items, checkItem{Info: true, Label: m.Name + ": " + rc.Label + " (optional)"})
			default:
				items = append(items, checkItem{Label: m.Name + ": " + rc.Label, Fix: rc.Fix})
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

	// Cite the source of each provider's extra criteria, so the checklist reads as
	// "X's own best practices", not sloop's opinion.
	seen := map[string]bool{}
	for _, t := range proj.Tools {
		m := manifests[t]
		if m.Readiness.Docs != "" && !seen[m.Readiness.Docs] {
			seen[m.Readiness.Docs] = true
			_, _ = fmt.Fprintf(w, "%s %s — %s\n", tui.Grey("  learn more:"), m.Name, tui.Grey(m.Readiness.Docs))
		}
	}
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
