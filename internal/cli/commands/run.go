package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/hints"
	"github.com/stroops/sloop/internal/runner"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
	"github.com/stroops/sloop/internal/workspace"
)

// launchPlan is the resolved outcome of a `sloop run` target + flags.
type launchPlan struct {
	toolKey string // a real manifest key (binary aliases already resolved)
	model   string // model to forward ("" = the CLI's own default)
	effort  string // low|medium|high ("" = none)
}

// toolKeyFor resolves a token to a manifest key: an exact key, else a tool whose
// detect/launch binary matches — so `sloop run agent` == `sloop run cursor`.
func toolKeyFor(token string, manifests map[string]adapter.Manifest) (string, bool) {
	if _, ok := manifests[token]; ok {
		return token, true
	}
	for key, m := range manifests {
		if m.Detect == token || filepath.Base(m.Launch) == token {
			return key, true
		}
	}
	return "", false
}

// modelHomeTool finds the CLI that should launch a bare model alias: the tool
// listing it in run.models that is the canonical launcher for its own vendor
// (run.default_for). Falls back to any tool that serves the alias.
func modelHomeTool(model string, manifests map[string]adapter.Manifest) (string, bool) {
	var fallback string
	for key, m := range manifests {
		for _, a := range m.Run.Models {
			if a != model {
				continue
			}
			if m.Run.Vendor != "" && sliceHas(m.Run.DefaultFor, m.Run.Vendor) {
				return key, true
			}
			if fallback == "" {
				fallback = key
			}
		}
	}
	return fallback, fallback != ""
}

func sliceHas(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// planLaunch resolves the positional target + flags into a CLI, model, and
// effort. Precedence for the positional token: tool key → binary alias → model
// alias. Naming a CLI (positional tool or --provider) wins; a bare model
// resolves to its vendor's home CLI.
func planLaunch(target, provider, model, effort, defaultTool string, manifests map[string]adapter.Manifest) (launchPlan, error) {
	p := launchPlan{model: model, effort: effort}
	cli := provider

	if target != "" {
		switch {
		case isToolToken(target, manifests):
			key, _ := toolKeyFor(target, manifests)
			if cli != "" {
				if k2, _ := toolKeyFor(cli, manifests); k2 != key {
					return p, fmt.Errorf("conflicting tools: %q and --provider %q", target, provider)
				}
			}
			cli = key
		default:
			home, ok := modelHomeTool(target, manifests)
			if !ok {
				return p, fmt.Errorf("unknown tool or model %q (try `sloop run <tool> -m <model>`)", target)
			}
			if p.model == "" {
				p.model = target
			}
			if cli == "" {
				cli = home
			}
		}
	}

	if cli == "" {
		cli = defaultTool
	}
	if cli == "" {
		return p, fmt.Errorf("no tool given and no default_tool set")
	}
	key, ok := toolKeyFor(cli, manifests)
	if !ok {
		return p, fmt.Errorf("unknown tool %q (no adapter)", cli)
	}
	p.toolKey = key
	return p, nil
}

func isToolToken(token string, manifests map[string]adapter.Manifest) bool {
	_, ok := toolKeyFor(token, manifests)
	return ok
}

// buildRunArgs turns a resolved model/effort into the CLI's own flags (from its
// manifest), then appends passthrough. Errors clearly if the CLI lacks a knob
// the user asked for. The model string is forwarded as-is — never validated.
func buildRunArgs(m adapter.Manifest, model, effort string, passthrough []string) ([]string, error) {
	var args []string
	if model != "" {
		if m.Run.ModelFlag == "" {
			return nil, fmt.Errorf("%s has no model selection via sloop — run it and pick the model inside, or pass flags after `--`", m.Name)
		}
		args = append(args, m.Run.ModelFlag, model)
	}
	if effort != "" {
		switch effort {
		case "low", "medium", "high":
		default:
			return nil, fmt.Errorf("--effort must be low|medium|high, got %q", effort)
		}
		tok, ok := m.Run.EffortValues[effort]
		if m.Run.EffortFlag == "" || !ok {
			return nil, fmt.Errorf("%s has no %q reasoning effort via sloop", m.Name, effort)
		}
		args = append(args, m.Run.EffortFlag, tok)
	}
	return append(args, passthrough...), nil
}

// RunRun resolves the target + model/effort flags, syncs context, then launches
// the CLI with the right flags in a managed session.
func RunRun(startDir, target, provider, model, effort string, passthrough []string, r runner.Runner) error {
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
	plan, err := planLaunch(target, provider, model, effort, proj.DefaultTool, manifests)
	if err != nil {
		return err
	}
	m := manifests[plan.toolKey]
	args, err := buildRunArgs(m, plan.model, plan.effort, passthrough)
	if err != nil {
		return err
	}

	// Sync native files before launch.
	if _, err := RunSync(startDir, plan.toolKey, false); err != nil {
		return err
	}

	// Record session (best-effort: never block the launch).
	sessID, store := recordSessionBestEffort(ws, plan.toolKey, target)
	if store != nil {
		defer func() { _ = store.Close() }()
	}

	launchErr := r.Launch(runner.Spec{Dir: ws.Root, Command: m.Launch, Args: args})

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
		_ = store.Close()
		return 0, nil
	}
	id, err := store.RecordSession(session.Session{
		WorkspaceID: w.ID, Tool: tool, Profile: profileName, Cwd: ws.Root, StartedAt: time.Now(),
	})
	if err != nil {
		_ = store.Close()
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
	defer func() { _ = store.Close() }()
	ws, err := store.WorkspaceByName(workspaceFlag)
	if err != nil {
		return "", fmt.Errorf("workspace %q not found in registry: %w", workspaceFlag, err)
	}
	return ws.Path, nil
}

// selectRunner returns a tmux-backed runner when tmux is available, else exec.
func selectRunner(workspace, tool string) runner.Runner {
	if tmux.Available() {
		return tmux.Runner{Session: tmux.SessionName(workspace, tool)}
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
	if !tmux.Available() {
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
	session := tmux.SessionName(ws.Name, strings.Join(tools, "_"))
	return tmux.LaunchSplit(session, ws.Root, cmds)
}

var (
	runWorkspace string
	runSplit     bool
	runProvider  string
	runModel     string
	runEffort    string
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
			if err := RunSplit(startDir, tools); err != nil {
				return err
			}
			hints.Show(cmd.OutOrStdout(), "run")
			return nil
		}
		target := ""
		if len(positional) == 1 {
			target = positional[0]
		}
		// Resolve the plan up front so the session is named by the real tool key
		// (e.g. `run agent`/`run opus` both land in the canonical cursor/claude
		// session), not the raw token.
		manifests, err := adapter.Load()
		if err != nil {
			return err
		}
		plan, err := planLaunch(target, runProvider, runModel, runEffort, proj.DefaultTool, manifests)
		if err != nil {
			return err
		}
		if err := RunRun(startDir, target, runProvider, runModel, runEffort, passthrough, selectRunner(ws.Name, plan.toolKey)); err != nil {
			return err
		}
		hints.Show(cmd.OutOrStdout(), "run")
		return nil
	},
}

func RegisterRun(cmd *cobra.Command) {
	runCmd.Flags().StringVarP(&runWorkspace, "workspace", "w", "", "target a registered workspace by name")
	runCmd.Flags().BoolVar(&runSplit, "split", false, "launch multiple tools side-by-side as tmux panes")
	runCmd.Flags().StringVarP(&runProvider, "provider", "p", "", "AI CLI to launch (overrides the positional target)")
	runCmd.Flags().StringVarP(&runModel, "model", "m", "", "model to pass to the CLI (forwarded as-is, not validated)")
	runCmd.Flags().StringVarP(&runEffort, "effort", "e", "", "reasoning effort: low|medium|high (if the CLI supports it)")
	runCmd.ValidArgsFunction = completeTools
	_ = runCmd.RegisterFlagCompletionFunc("workspace", completeWorkspaces)
	_ = runCmd.RegisterFlagCompletionFunc("provider", completeTools)
	_ = runCmd.RegisterFlagCompletionFunc("model", completeModels)
	cmd.AddCommand(runCmd)
}
