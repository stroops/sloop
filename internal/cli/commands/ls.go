package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
	"github.com/stroops/sloop/internal/tui"
	"github.com/stroops/sloop/internal/workspace"
)

// liveByWorkspace groups live tmux fleet sessions by workspace name, so `ls` can
// show which registered workspaces currently have an agent running.
func liveByWorkspace() map[string][]FleetRow {
	m := map[string][]FleetRow{}
	for _, r := range fleetRows(tmux.ParseSessions(tmuxList())) {
		m[r.Workspace] = append(m[r.Workspace], r)
	}
	return m
}

// launchWorkspaceDefault launches a workspace's default tool in its own dir — the
// "jump into work" action for a workspace that has no live session yet.
func launchWorkspaceDefault(startDir string) error {
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
	plan, err := planLaunch("", "", "", "", proj.DefaultTool, manifests)
	if err != nil {
		return err
	}
	return RunRun(startDir, "", "", "", "", "", nil, selectRunner(ws.Name, plan.toolKey))
}

func RunLs(w io.Writer) error {
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	workspaces, err := store.ListWorkspaces()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "Workspaces:")
	for _, ws := range workspaces {
		_, _ = fmt.Fprintf(w, "  %-16s %s\n", ws.Name, ws.Path)
	}

	sessions, err := store.ListSessions(10)
	if err != nil {
		return err
	}
	if len(sessions) > 0 {
		_, _ = fmt.Fprintln(w, "Recent sessions:")
		for _, s := range sessions {
			_, _ = fmt.Fprintf(w, "  %s  tool=%s  %s\n", s.StartedAt.Format("2006-01-02 15:04"), s.Tool, s.Cwd)
		}
	}
	return nil
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List sloop workspaces and recent sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := config.GlobalDBPath()
		if err != nil {
			return err
		}
		store, err := session.Open(dbPath)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()

		workspaces, err := store.ListWorkspaces()
		if err != nil {
			return err
		}
		if len(workspaces) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No workspaces found.")
			return nil
		}

		nameW := 0
		for _, ws := range workspaces {
			if len(ws.Name) > nameW {
				nameW = len(ws.Name)
			}
		}

		live := liveByWorkspace()
		var options []string
		for _, ws := range workspaces {
			status := tui.Grey("idle")
			if n := len(live[ws.Name]); n > 0 {
				status = tui.Green(fmt.Sprintf("● %d live", n))
			}
			options = append(options, fmt.Sprintf("%-*s  %s  %s", nameW, ws.Name, status, tui.Grey(ws.Path)))
		}

		prompt := fmt.Sprintf("⚓ Sloop workspaces · %d", len(workspaces)) +
			"\r\n" + tui.Grey("  ↑/↓ move · ⏎ attach/show path · r launch tool · q quit")
		// Enter is safe (attach a live session, or just show the path); launching a
		// tool is the explicit `r` action, so Enter never spawns an agent by surprise.
		selected, key, err := tui.SelectAction(prompt, options, []byte{'r'})
		if err != nil {
			return err
		}
		if selected < 0 || key == 0 {
			return nil
		}
		ws := workspaces[selected]
		switch key {
		case 'r':
			return launchWorkspaceDefault(ws.Path)
		default: // Enter
			if rows := live[ws.Name]; len(rows) > 0 {
				return attachSession(rows[0].Name)
			}
			fmt.Printf("\n%s has no running agent. Open it with:\n  cd %s\nor press r in the menu to launch its default tool.\n", ws.Name, ws.Path)
			return nil
		}
	},
}

func RegisterLs(cmd *cobra.Command) { cmd.AddCommand(lsCmd) }
