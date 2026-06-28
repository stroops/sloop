package commands

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
)

// restoreTarget is one agent to bring back: a workspace + tool, and the tmux
// session name it will reclaim.
type restoreTarget struct {
	WSName  string
	Path    string
	Tool    string
	Session string
}

// restoreCandidates picks, from session history (most-recent first), the latest
// session per (workspace, tool) that is not currently live — i.e. the recent
// fleet to relaunch after a restart. Pure and testable. Sessions whose workspace
// is no longer in the registry are skipped.
func restoreCandidates(sessions []session.Session, wsName, wsPath map[int64]string, live map[string]bool) []restoreTarget {
	seen := map[string]bool{}
	var out []restoreTarget
	for _, s := range sessions {
		name, path := wsName[s.WorkspaceID], wsPath[s.WorkspaceID]
		if name == "" || path == "" {
			continue
		}
		key := name + "\x00" + s.Tool
		if seen[key] {
			continue
		}
		seen[key] = true
		sess := tmux.SessionName(name, s.Tool)
		if live[sess] {
			continue // already running — nothing to restore
		}
		out = append(out, restoreTarget{WSName: name, Path: path, Tool: s.Tool, Session: sess})
	}
	return out
}

// liveSessionNames is the set of tmux session names currently running.
func liveSessionNames() map[string]bool {
	live := map[string]bool{}
	for _, s := range tmux.ParseSessions(tmuxList()) {
		live[s.Name] = true
	}
	return live
}

// RunRestore relaunches the recent agents that aren't currently running, each in
// a detached session (so the whole fleet comes back at once). With resume it
// passes each tool's resume flag to continue the prior conversation.
func RunRestore(w io.Writer, in io.Reader, resume, yes bool) error {
	if !tmux.Available() {
		return fmt.Errorf("tmux is not installed; `sloop restore` needs tmux")
	}
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	sessions, err := store.ListSessions(100)
	if err != nil {
		return err
	}
	workspaces, err := store.ListWorkspaces()
	if err != nil {
		return err
	}
	wsName, wsPath := map[int64]string{}, map[int64]string{}
	for _, ws := range workspaces {
		wsName[ws.ID], wsPath[ws.ID] = ws.Name, ws.Path
	}

	targets := restoreCandidates(sessions, wsName, wsPath, liveSessionNames())
	if len(targets) == 0 {
		_, _ = fmt.Fprintln(w, "Nothing to restore — your recent agents are already running, or none are recorded yet.")
		return nil
	}

	_, _ = fmt.Fprintln(w, "Restore these agents:")
	for _, t := range targets {
		_, _ = fmt.Fprintf(w, "  %s  %s\n", t.Session, t.Path)
	}
	if !yes && !confirm(w, in, fmt.Sprintf("relaunch %d agent(s)? [y/N] ", len(targets))) {
		return nil
	}

	manifests, _ := adapter.Load()
	done := 0
	for _, t := range targets {
		m, ok := manifests[t.Tool]
		if !ok {
			_, _ = fmt.Fprintf(w, "  ✗ %s: no adapter for %q\n", t.Session, t.Tool)
			continue
		}
		_, _ = RunSync(t.Path, t.Tool, false)
		var args []string
		if resume && m.Run.ResumeFlag != "" {
			args = append(args, m.Run.ResumeFlag)
		}
		if err := tmux.LaunchDetached(t.Session, t.Path, m.Launch, args); err != nil {
			_, _ = fmt.Fprintf(w, "  ✗ %s: %v\n", t.Session, err)
			continue
		}
		if ws, err := store.RegisterWorkspace(t.WSName, t.Path); err == nil {
			_, _ = store.RecordSession(session.Session{
				WorkspaceID: ws.ID, Tool: t.Tool, Cwd: t.Path,
				TmuxSession: t.Session, StartedAt: time.Now(),
			})
		}
		done++
	}
	_, _ = fmt.Fprintf(w, "restored %d agent(s) — `sloop ps` to see them\n", done)
	return nil
}

var (
	restoreResume bool
	restoreYes    bool
)

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Relaunch the agents you were recently running (after a reboot / tmux restart)",
	Long: `tmux sessions don't survive a reboot. restore re-creates the most recent agent
per workspace that isn't currently running, from sloop's registry — detached, so
the whole fleet comes back at once. With --resume it continues each tool's prior
conversation where the CLI supports it.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunRestore(cmd.OutOrStdout(), cmd.InOrStdin(), restoreResume, assumeYes(cmd, restoreYes))
	},
}

func RegisterRestore(cmd *cobra.Command) {
	restoreCmd.Flags().BoolVar(&restoreResume, "resume", false, "continue each agent's prior conversation (where the CLI supports it)")
	restoreCmd.Flags().BoolVar(&restoreYes, "yes", false, "skip the confirmation (or use global -y)")
	cmd.AddCommand(restoreCmd)
}
