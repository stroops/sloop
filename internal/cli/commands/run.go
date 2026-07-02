package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/fleetstate"
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
// detect/launch binary matches, so `sloop run agent` == `sloop run cursor`.
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

// buildRunArgs turns a resolved model/effort/task into the CLI's own flags (from
// its manifest), then appends passthrough. Errors clearly if the CLI lacks a
// knob the user asked for. The model string is forwarded as-is, never validated.
func buildRunArgs(m adapter.Manifest, model, effort, task string, passthrough []string) ([]string, error) {
	var args []string
	if model != "" {
		if m.Run.ModelFlag == "" {
			return nil, fmt.Errorf("%s has no model selection via sloop; run it and pick the model inside, or pass flags after `--`", m.Name)
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
	if task != "" {
		switch m.Run.Prompt {
		case "":
			return nil, fmt.Errorf("%s has no initial-task support via sloop; launch it and type the task", m.Name)
		case "positional":
			args = append(args, task) // e.g. `claude "task"` / `agent "task"`
		default:
			args = append(args, m.Run.Prompt, task) // flag form: `<flag> <task>`
		}
	}
	return append(args, passthrough...), nil
}

// resolveAndLaunch resolves the target + model/effort/task flags against the
// workspace, syncs context, then launches the CLI in a managed session — the
// one-shot form (the tests' seam). `run`/`new`/`ls` resolve earlier themselves
// (the session must be named before a runner is picked) and call
// launchResolved directly, so nothing is loaded twice.
func resolveAndLaunch(startDir, target, provider, model, effort, task string, passthrough []string, env map[string]string, instance string, r runner.Runner) error {
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
	return launchResolved(startDir, ws, manifests[plan.toolKey], plan, task, passthrough, env, instance, r)
}

// launchResolved syncs context and launches an already-resolved plan in ws.
// env is injected into the launched process (e.g. a second account's
// CLAUDE_CONFIG_DIR), and instance records which named instance this session is.
func launchResolved(startDir string, ws *workspace.Workspace, m adapter.Manifest, plan launchPlan, task string, passthrough []string, env map[string]string, instance string, r runner.Runner) error {
	args, err := buildRunArgs(m, plan.model, plan.effort, task, passthrough)
	if err != nil {
		return err
	}

	// Sync native files before launch.
	if _, err := RunSync(startDir, plan.toolKey, false); err != nil {
		return err
	}

	// Record session (best-effort: never block the launch).
	sessID, store := recordSessionBestEffort(ws, plan.toolKey, instance)
	if store != nil {
		defer func() { _ = store.Close() }()
	}

	launchErr := r.Launch(runner.Spec{Dir: ws.Root, Command: m.Launch, Args: args, Env: env})

	// A runner that left a detached session running returns as soon as it is
	// created; the agent is still going, so its history row must stay open.
	if store != nil && sessID > 0 && !launchIsDetached(r) {
		_ = store.EndSession(sessID, time.Now())
	}
	return launchErr
}

func launchIsDetached(r runner.Runner) bool {
	d, ok := r.(runner.Detacher)
	return ok && d.Detached()
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
	return selectRunnerInstance(workspace, tool, "")
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

// launchFlags is the flag surface `run` and `new` share; each command binds
// its own instance via addLaunchFlags so the two can never drift apart.
type launchFlags struct {
	workspace string
	provider  string
	model     string
	effort    string
	task      string
	name      string
	env       []string
	fresh     bool // --new: take the next free instance slot instead of reusing the session
}

var (
	runFlags launchFlags
	runSplit bool
)

// addLaunchFlags registers the shared launch flags (and their completions)
// on a command, bound to f.
func addLaunchFlags(c *cobra.Command, f *launchFlags) {
	c.Flags().StringVarP(&f.workspace, "workspace", "w", "", "target a registered workspace by name")
	c.Flags().StringVarP(&f.provider, "provider", "p", "", "AI CLI to launch (overrides the positional target)")
	c.Flags().StringVarP(&f.model, "model", "m", "", "model to pass to the CLI (forwarded as-is, not validated)")
	c.Flags().StringVarP(&f.effort, "effort", "e", "", "reasoning effort: low|medium|high (if the CLI supports it)")
	c.Flags().StringVarP(&f.task, "task", "t", "", "hand the agent an initial task; the session starts already working on it")
	c.Flags().StringVarP(&f.name, "name", "n", "", "name this instance so a second agent of the same tool gets its own session")
	c.Flags().StringArrayVar(&f.env, "env", nil, "extra env for the launched tool, KEY=VAL (e.g. a second account's CLAUDE_CONFIG_DIR); repeatable")
	c.Flags().BoolVarP(&f.fresh, "new", "N", false, "start a fresh instance (next free slot) instead of reusing the existing session")
	c.ValidArgsFunction = completeTools
	_ = c.RegisterFlagCompletionFunc("workspace", completeWorkspaces)
	_ = c.RegisterFlagCompletionFunc("provider", completeTools)
	_ = c.RegisterFlagCompletionFunc("model", completeModels)
}

// splitDashArgs splits cobra args at `--`: tool/profile targets before it,
// passthrough for the launched tool after it.
func splitDashArgs(cmd *cobra.Command, args []string) (positional, passthrough []string) {
	if d := cmd.ArgsLenAtDash(); d >= 0 {
		return args[:d], args[d:]
	}
	return args, nil
}

// maxOneTarget rejects more than one tool/profile token before `--`; hint
// names the command's escape hatch for multiple, if it has one.
func maxOneTarget(cmd *cobra.Command, args []string, hint string) error {
	n := len(args)
	if d := cmd.ArgsLenAtDash(); d >= 0 {
		n = d // only the args before -- count as tool/profile targets
	}
	if n > 1 {
		return fmt.Errorf("accepts at most 1 tool/profile argument before --%s", hint)
	}
	return nil
}

// launchSettings is launchFlags plus the one thing the commands disagree on:
// whether the launch attaches.
type launchSettings struct {
	launchFlags
	detached bool // create the session without attaching (`sloop new`)
}

// executeLaunch is the shared body of `sloop run` and `sloop new`: resolve the
// target + flags into a tool/session, then launch it attached or detached.
func executeLaunch(cmd *cobra.Command, positional, passthrough []string, set launchSettings) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	startDir, err := resolveStartDir(cwd, set.workspace)
	if err != nil {
		return err
	}
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return err
	}
	if set.detached && !tmux.Available() {
		return fmt.Errorf("sloop new requires tmux (the detached session lives in tmux); plain `sloop run` works without it")
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
	// Interpret @profile / tool@instance and fold in --name/--env, then plan
	// against the resolved tool token. Profiles live in the global config, only
	// loaded when the target actually names one.
	var profiles map[string]config.Profile
	if strings.Contains(target, "@") {
		glob, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		profiles = glob.Profiles
	}
	res, err := resolveInstance(target, set.name, set.env, profiles, manifests)
	if err != nil {
		return err
	}
	// When running via a profile, confirm which tool + env are active so the
	// user can verify the right account/config is being used.
	if strings.HasPrefix(target, "@") && len(res.env) > 0 {
		envKeys := make([]string, 0, len(res.env))
		for k := range res.env {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		cmd.Printf("profile %q: %s  env: %s\n", res.instance, res.target, strings.Join(envKeys, ", "))
	}
	plan, err := planLaunch(res.target, set.provider, set.model, set.effort, proj.DefaultTool, manifests)
	if err != nil {
		return err
	}
	// --new spins a fresh instance (next free slot) instead of re-attaching;
	// it's a no-op once a name/instance already makes the session distinct.
	if set.fresh && res.instance == "" {
		res.instance = nextFreeInstance(ws.Name, plan.toolKey, tmux.ParseSessions(tmuxList()))
	}
	sessName := tmux.InstanceName(ws.Name, plan.toolKey, res.instance)
	// `new` without --new is "ensure this session exists, stay outside": when
	// it's already running there is nothing to create or sync (--new always
	// lands on a free slot, so the check would be a wasted subprocess).
	if set.detached && !set.fresh && tmux.HasSession(sessName) {
		cmd.Println(tmux.AlreadyRunningMsg(sessName))
		return nil
	}
	// Record the requested model so the status bar can show it even for
	// tools with no statusline feed; a feed later overwrites it with the
	// provider's own display name. Best-effort, never blocks the launch.
	if plan.model != "" {
		_ = fleetstate.WriteInfo(sessName, plan.model, 0)
	}
	var r runner.Runner
	if set.detached {
		r = &tmux.DetachedRunner{Session: sessName, Out: cmd.OutOrStdout()}
	} else {
		r = selectRunnerInstance(ws.Name, plan.toolKey, res.instance)
	}
	if err := launchResolved(startDir, ws, manifests[plan.toolKey], plan, set.task, passthrough, res.env, res.instance, r); err != nil {
		return err
	}
	hints.Show(cmd.OutOrStdout(), "run")
	return nil
}

var runCmd = &cobra.Command{
	Use:     "run [tool | @profile | tool@instance] [-- <args>]",
	Aliases: []string{"r"},
	Short:   "Sync context and launch an AI tool in the workspace (--split for side-by-side panes)",
	Long: `Sync context and launch an AI tool in the workspace.

Run a second agent of the same provider with a named instance, or a different
account with a saved profile:

  sloop run claude            # the workspace's claude (re-attaches if running)
  sloop run claude --new      # a fresh claude instance (claude·2, claude·3, …)
  sloop run claude@review     # an ad-hoc instance named "review"
  sloop run @work             # the "work" profile (e.g. a second account)

Profiles live in ~/.sloop/config.yaml; manage them with ` + "`sloop profile`" + `.
Use --env KEY=VAL for a one-off account/env without saving a profile.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if runSplit {
			return nil // each positional is one pane
		}
		return maxOneTarget(cmd, args, " (use --split for multiple)")
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		positional, passthrough := splitDashArgs(cmd, args)
		if runSplit {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			startDir, err := resolveStartDir(cwd, runFlags.workspace)
			if err != nil {
				return err
			}
			tools := positional
			if len(tools) == 0 {
				ws, err := workspace.Resolve(startDir)
				if err != nil {
					return err
				}
				proj, err := config.LoadProject(ws.SloopDir())
				if err != nil {
					return err
				}
				tools = []string{proj.DefaultTool}
			}
			if err := RunSplit(startDir, tools); err != nil {
				return err
			}
			hints.Show(cmd.OutOrStdout(), "run")
			return nil
		}
		return executeLaunch(cmd, positional, passthrough, launchSettings{launchFlags: runFlags})
	},
}

func RegisterRun(cmd *cobra.Command) {
	addLaunchFlags(runCmd, &runFlags)
	runCmd.Flags().BoolVar(&runSplit, "split", false, "launch multiple tools side-by-side as tmux panes")
	cmd.AddCommand(runCmd)
}
