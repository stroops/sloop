package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/runner"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/workspace"
)

func RunRun(startDir, target string, r runner.Runner) error {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return err
	}
	prof, err := resolveProfile(ws.SloopDir(), target, proj.DefaultTool)
	if err != nil {
		return err
	}
	manifests, err := adapter.LoadBuiltin()
	if err != nil {
		return err
	}
	m, ok := manifests[prof.Tool]
	if !ok {
		return fmt.Errorf("unknown tool %q (no adapter)", prof.Tool)
	}

	// Sync native files before launch.
	if _, err := RunSync(startDir, target); err != nil {
		return err
	}

	// Record session (best-effort: never block the launch).
	sessID, store := recordSessionBestEffort(ws, prof.Tool, target)
	if store != nil {
		defer store.Close()
	}

	launchErr := r.Launch(runner.Spec{Dir: ws.Root, Command: m.Launch})

	if store != nil && sessID > 0 {
		_ = store.EndSession(sessID, time.Now())
	}
	return launchErr
}

func recordSessionBestEffort(ws *workspace.Workspace, tool, profileName string) (int64, *session.Store) {
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return 0, nil
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return 0, nil
	}
	w, err := store.RegisterWorkspace(ws.Name, ws.Root)
	if err != nil {
		store.Close()
		return 0, nil
	}
	id, err := store.RecordSession(session.Session{
		WorkspaceID: w.ID, Tool: tool, Profile: profileName, Cwd: ws.Root, StartedAt: time.Now(),
	})
	if err != nil {
		store.Close()
		return 0, nil
	}
	return id, store
}

var runCmd = &cobra.Command{
	Use:   "run [tool|profile]",
	Short: "Sync context and launch an AI tool in the workspace",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		target := ""
		if len(args) == 1 {
			target = args[0]
		}
		return RunRun(cwd, target, runner.ExecRunner{})
	},
}

func RegisterRun(cmd *cobra.Command) { cmd.AddCommand(runCmd) }
