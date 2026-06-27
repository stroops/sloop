package commands

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/runner"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/workspace"
)

func RunRun(startDir, target string, extraArgs []string, r runner.Runner) error {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return err
	}
	tool, err := resolveTool(target, proj.DefaultTool)
	if err != nil {
		return err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	m, ok := manifests[tool]
	if !ok {
		return fmt.Errorf("unknown tool %q (no adapter)", tool)
	}

	// Sync native files before launch.
	if _, err := RunSync(startDir, target, false); err != nil {
		return err
	}

	// Record session (best-effort: never block the launch).
	sessID, store := recordSessionBestEffort(ws, tool, target)
	if store != nil {
		defer store.Close()
	}

	launchErr := r.Launch(runner.Spec{Dir: ws.Root, Command: m.Launch, Args: extraArgs})

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

// resolveStartDir maps an optional -w workspace name to a directory via the
// registry; with no flag it returns cwd unchanged.
func resolveStartDir(cwd, workspaceFlag string) (string, error) {
	if workspaceFlag == "" {
		return cwd, nil
	}
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return "", err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer store.Close()
	ws, err := store.WorkspaceByName(workspaceFlag)
	if err != nil {
		return "", fmt.Errorf("workspace %q not found in registry: %w", workspaceFlag, err)
	}
	return ws.Path, nil
}

// selectRunner returns a tmux-backed runner when tmux is available, else exec.
func selectRunner(workspace, tool string) runner.Runner {
	if runner.TmuxAvailable() {
		return runner.TmuxRunner{Session: runner.TmuxSessionName(workspace, tool)}
	}
	return runner.ExecRunner{}
}

// RunSplit syncs each tool then launches them side-by-side as tmux panes in one
// session, all rooted at the workspace. Requires tmux.
func RunSplit(startDir string, tools []string) error {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return err
	}
	if !runner.TmuxAvailable() {
		return fmt.Errorf("--split requires tmux (each pane is a tmux pane); plain `sloop run` works without it")
	}
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	var cmds []string
	for _, t := range tools {
		m, ok := manifests[t]
		if !ok {
			return fmt.Errorf("unknown tool %q (no adapter)", t)
		}
		if _, err := RunSync(startDir, t, false); err != nil {
			return err
		}
		cmds = append(cmds, m.Launch)
	}
	session := runner.TmuxSessionName(ws.Name, strings.Join(tools, "_"))
	return runner.LaunchSplit(session, ws.Root, cmds)
}

var (
	runWorkspace string
	runSplit     bool
)

var runCmd = &cobra.Command{
	Use:     "run [tool|profile ...] [-- <args>]",
	Aliases: []string{"r"},
	Short:   "Sync context and launch an AI tool in the workspace (--split for side-by-side panes)",
	Args: func(cmd *cobra.Command, args []string) error {
		n := len(args)
		if d := cmd.ArgsLenAtDash(); d >= 0 {
			n = d // only the args before -- count as tool/profile targets
		}
		if !runSplit && n > 1 {
			return fmt.Errorf("accepts at most 1 tool/profile argument before -- (use --split for multiple)")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		startDir, err := resolveStartDir(cwd, runWorkspace)
		if err != nil {
			return err
		}
		// Split "<tool> -- <args>": everything after -- is passed through to the tool.
		positional, passthrough := args, []string(nil)
		if d := cmd.ArgsLenAtDash(); d >= 0 {
			positional, passthrough = args[:d], args[d:]
		}
		ws, err := workspace.Resolve(startDir)
		if err != nil {
			return err
		}
		proj, err := config.LoadProject(ws.SloopDir())
		if err != nil {
			return err
		}
		if runSplit {
			tools := positional
			if len(tools) == 0 {
				tools = []string{proj.DefaultTool}
			}
			return RunSplit(startDir, tools)
		}
		target := ""
		if len(positional) == 1 {
			target = positional[0]
		}
		if target == "" {
			target = proj.DefaultTool
		}
		return RunRun(startDir, target, passthrough, selectRunner(ws.Name, target))
	},
}

func RegisterRun(cmd *cobra.Command) {
	runCmd.Flags().StringVarP(&runWorkspace, "workspace", "w", "", "target a registered workspace by name")
	runCmd.Flags().BoolVar(&runSplit, "split", false, "launch multiple tools side-by-side as tmux panes")
	runCmd.ValidArgsFunction = completeTools
	_ = runCmd.RegisterFlagCompletionFunc("workspace", completeWorkspaces)
	cmd.AddCommand(runCmd)
}
