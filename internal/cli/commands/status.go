package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/tmux"
	"github.com/stroops/sloop/internal/tui"
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
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "⚓ %s  %s\n", tui.Bold(ws.Name), tui.Grey(ws.Root))

	// tools (default marked) + which auto-install hooks + which have skills linked.
	var tools, autoHooks, linked []string
	for _, t := range proj.Tools {
		label := t
		if t == proj.DefaultTool {
			label = tui.Bold(t + "*")
		}
		tools = append(tools, label)
		m := manifests[t]
		if m.Hooks.Install == "settings-json" {
			autoHooks = append(autoHooks, t)
		}
		if m.Skills.Target != "" && syncpkg.SkillsState(ws.Root, ws.SloopDir(), m) == "linked" {
			linked = append(linked, t)
		}
	}
	fmt.Fprintf(w, "  tools:    %s  %s\n", strings.Join(tools, ", "), tui.Grey("(* default)"))

	// context: AGENTS.md + the default tool's pointer state.
	dm := manifests[proj.DefaultTool]
	fmt.Fprintf(w, "  context:  AGENTS.md %s · %s %s\n",
		stateMark(syncpkg.AgentsState(ws.Root)),
		ctxName(dm), stateMark(syncpkg.ContextState(ws.Root, dm)))

	// skills.
	linkedStr := "none"
	if len(linked) > 0 {
		linkedStr = strings.Join(linked, ", ")
	}
	fmt.Fprintf(w, "  skills:   %d in .sloop/skills · linked: %s\n", skillCount(ws.SloopDir()), linkedStr)

	// hooks.
	hooksStr := tui.Grey("none installed")
	if len(autoHooks) > 0 {
		hooksStr = "auto: " + strings.Join(autoHooks, ", ")
	}
	fmt.Fprintf(w, "  hooks:    %s  %s\n", hooksStr, tui.Grey("(sloop hooks list)"))

	// running sessions (best-effort; needs tmux).
	if tmux.Available() {
		n := 0
		for _, r := range fleetRows(tmux.ParseSessions(tmuxList())) {
			if r.Workspace == ws.Name {
				n++
			}
		}
		fmt.Fprintf(w, "  running:  %d sessions  %s\n", n, tui.Grey("(sloop ps)"))
	}
	return nil
}

// stateMark colors a delivery state: healthy states green, anything else yellow.
func stateMark(s string) string {
	switch s {
	case "ok", "linked", "native":
		return tui.Green(s)
	default:
		return tui.Yellow(s)
	}
}

func ctxName(m adapter.Manifest) string {
	if m.Context.Mode == "pointer" && m.Context.File != "" {
		return m.Context.File
	}
	return "native"
}

func skillCount(sloopDir string) int {
	entries, err := os.ReadDir(filepath.Join(sloopDir, "skills"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			n++
		}
	}
	return n
}

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st"},
	Short:   "Show the current sloop workspace status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return RunStatus(cwd, cmd.OutOrStdout())
	},
}

func RegisterStatus(cmd *cobra.Command) { cmd.AddCommand(statusCmd) }
