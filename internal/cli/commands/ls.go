package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
	"github.com/stroops/sloop/internal/tui"
	"github.com/stroops/sloop/internal/workspace"
)

// openShellIn starts the user's shell with its working directory set to dir — a
// real "go there" for a workspace without a session (a binary can't cd the
// parent shell). Exiting the shell returns to `sloop ls`.
func openShellIn(dir string) error {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	fmt.Printf("\nshell in %s — type `exit` to return to sloop\n\n", dir)
	c := exec.Command(sh)
	c.Dir = dir
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// enrichedByWorkspace groups the live fleet by workspace name, classified
// (waiting/working/idle) so `ls` can show each workspace's agents with the same
// status dots as `ps` — the bridge between the two views.
func enrichedByWorkspace(manifests map[string]adapter.Manifest) map[string][]FleetRow {
	m := map[string][]FleetRow{}
	for _, r := range enrichGlances(fleetRows(tmux.ParseSessions(tmuxList()), manifests), manifests) {
		m[r.Workspace] = append(m[r.Workspace], r)
	}
	return m
}

// workspaceDefault returns a workspace's default tool key (what `r`/launch runs),
// or "—" when it can't be read.
func workspaceDefault(path string) string {
	proj, err := config.LoadProject(workspace.Workspace{Root: path}.SloopDir())
	if err != nil || proj.DefaultTool == "" {
		return "—"
	}
	return proj.DefaultTool
}

// agentsInline renders a workspace's live agents as "● Claude  ▸ Cursor" using
// the same status dots as `ps`; "—" when nothing is running.
func agentsInline(rows []FleetRow) string {
	if len(rows) == 0 {
		return tui.Grey("—")
	}
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		dot, _ := statusDot(r)
		parts = append(parts, dot+" "+r.toolName())
	}
	return strings.Join(parts, "  ")
}

// abbrevHome shortens a path under $HOME to a leading ~ for compact display.
func abbrevHome(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+"/") {
			return "~" + path[len(home):]
		}
	}
	return path
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
	return RunRun(startDir, "", "", "", "", "", nil, nil, "", selectRunner(ws.Name, plan.toolKey))
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

var lsPrune bool

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

		if lsPrune {
			pruned, err := store.PruneWorkspaces()
			if err != nil {
				return err
			}
			if len(pruned) == 0 {
				cmd.Println("Nothing to prune — all registered workspace paths exist.")
			} else {
				for _, name := range pruned {
					cmd.Printf("pruned: %s\n", name)
				}
			}
			return nil
		}

		workspaces, err := store.ListWorkspaces()
		if err != nil {
			return err
		}
		if len(workspaces) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No workspaces found.")
			return nil
		}

		manifests, _ := adapter.Load()

		// Per-workspace columns. The default tool doesn't change between keystrokes,
		// so compute it once; live agents are re-read each pass. The path goes on a
		// second `└` line (like ps), so only name + default need width here.
		defaults := make(map[string]string, len(workspaces))
		nameW, defW := len("WORKSPACE"), len("DEFAULT")
		for _, ws := range workspaces {
			defaults[ws.Name] = workspaceDefault(ws.Path)
			if len(ws.Name) > nameW {
				nameW = len(ws.Name)
			}
			if len(defaults[ws.Name]) > defW {
				defW = len(defaults[ws.Name])
			}
		}

		// Same control-center shape as `ps`: arrow-key move + action keys, looping
		// so light actions (cd, open a shell) return to the list. The screen is
		// cleared each pass and feedback shows in `notice` under the header, so the
		// list redraws in place instead of stacking. Enter stays safe (attach a live
		// session, else just point at the workspace); launching is the explicit `r`
		// key, so the list never spawns an agent by surprise.
		var notice string
		for {
			tui.Clear()
			live := enrichedByWorkspace(manifests)
			running := 0
			var options []string
			for _, ws := range workspaces {
				agents := live[ws.Name]
				if len(agents) > 0 {
					running++
				}
				// No leading status glyph (ambiguous-width breaks alignment); the
				// AGENTS column carries liveness — colored dots when running, "—" idle.
				line := fmt.Sprintf("%-*s %-*s %s",
					nameW, ws.Name, defW, defaults[ws.Name], agentsInline(agents))
				line += "\r\n└ " + tui.Grey(abbrevHome(ws.Path))
				options = append(options, line)
			}

			header := fmt.Sprintf("⚓ Workspaces · %d registered", len(workspaces))
			if running > 0 {
				header += " · " + tui.Green(fmt.Sprintf("%d running", running))
			}
			legend := tui.Grey("  AGENTS = live agents, colored by status (→ sloop ps) · — = idle")
			keys := tui.Grey("  ↑/↓ move · ⏎ open · r launch · s shell · c cd · q/esc quit")
			cols := tui.Grey(fmt.Sprintf("  %-*s %-*s %s", nameW, "WORKSPACE", defW, "DEFAULT", "AGENTS"))
			prompt := header
			if notice != "" {
				prompt += "\r\n" + notice
			}
			prompt += "\r\n" + legend + "\r\n" + keys + "\r\n\r\n" + cols
			notice = "" // consumed; the handler below sets a fresh one for next pass

			selected, key, err := tui.SelectAction(prompt, options, []byte{'r', 's', 'c'})
			if err != nil {
				return err
			}
			if selected < 0 || key == 0 {
				return nil
			}
			ws := workspaces[selected]
			switch key {
			case 'r': // launch the default tool (replaces this process via tmux)
				return launchWorkspaceDefault(ws.Path)
			case 's': // open a shell in the workspace dir, then return to the list
				if err := openShellIn(ws.Path); err != nil {
					return err
				}
			case 'c': // surface the cd line (a binary can't change the parent shell)
				notice = tui.Cyan("cd " + ws.Path)
			default: // Enter
				if rows := live[ws.Name]; len(rows) > 0 {
					return attachSession(rows[0].Name)
				}
				notice = tui.Yellow(ws.Name+" isn't running") +
					tui.Grey(" — press r to launch · s for a shell · c to copy its cd")
			}
		}
	},
}

func RegisterLs(cmd *cobra.Command) {
	lsCmd.Flags().BoolVar(&lsPrune, "prune", false, "remove workspace registrations whose paths no longer exist on disk")
	cmd.AddCommand(lsCmd)
}
