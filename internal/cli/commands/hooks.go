package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/workspace"
)

// sloopHooks maps each Claude hook event to the sloop status it records.
// UserPromptSubmit → the agent started working; Notification → it's blocked on
// you; Stop → it finished the turn (idle).
var sloopHooks = map[string]string{
	"UserPromptSubmit": "working",
	"Notification":     "waiting",
	"Stop":             "idle",
}

// hookCommandFor is the shell command Claude runs for an event.
func hookCommandFor(state string) string { return "sloop hook " + state }

// mergeClaudeHooks adds sloop's status hooks to a Claude settings JSON document
// (decoded into a generic map) without disturbing existing keys or other hooks.
// It returns the (possibly unchanged) document and whether anything was added.
func mergeClaudeHooks(root map[string]any) (map[string]any, bool) {
	if root == nil {
		root = map[string]any{}
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	changed := false
	for event, state := range sloopHooks {
		cmd := hookCommandFor(state)
		if hasCommandHook(hooks[event], cmd) {
			continue
		}
		group := map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": cmd}},
		}
		arr, _ := hooks[event].([]any)
		hooks[event] = append(arr, group)
		changed = true
	}
	root["hooks"] = hooks
	return root, changed
}

// hasCommandHook reports whether an event's hook groups already contain a
// command hook with the given command (so install is idempotent).
func hasCommandHook(eventVal any, cmd string) bool {
	groups, ok := eventVal.([]any)
	if !ok {
		return false
	}
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		hs, ok := gm["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range hs {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if c, _ := hm["command"].(string); c == cmd {
				return true
			}
		}
	}
	return false
}

// installClaudeHooks merges sloop's hooks into <root>/.claude/settings.local.json
// (a per-user, typically un-committed file). Returns the path and whether it changed.
func installClaudeHooks(root string) (string, bool, error) {
	claudeDir := filepath.Join(root, ".claude")
	path := filepath.Join(claudeDir, "settings.local.json")

	doc := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &doc); err != nil {
			return path, false, fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}
	merged, changed := mergeClaudeHooks(doc)
	if !changed {
		return path, false, nil
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return path, false, err
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return path, false, err
	}
	return path, true, os.WriteFile(path, append(out, '\n'), 0o644)
}

// providerHook describes how one AI tool exposes lifecycle hooks and where its
// config lives, so sloop's status integration is multi-provider aware. The
// event names below come from each tool's own docs; `sloop hook <state>` is the
// command they should call. Only Claude is auto-installed today (its format is
// verified); the rest are surfaced via `hooks print`/`list` so you can wire
// them by hand until each installer is added.
type providerHook struct {
	Tool        string
	ConfigPath  string
	DocsURL     string
	Working     string // event that means "started working"
	Waiting     string // event that means "blocked on you"
	Idle        string // event that means "turn finished"
	AutoInstall bool
	Notes       string
}

var providerHooks = []providerHook{
	{
		Tool: "claude", ConfigPath: ".claude/settings.local.json",
		DocsURL: "https://docs.claude.com/en/docs/claude-code/hooks",
		Working: "UserPromptSubmit", Waiting: "Notification", Idle: "Stop",
		AutoInstall: true,
	},
	{
		Tool: "gemini", ConfigPath: ".gemini/settings.json (or ~/.gemini/settings.json)",
		DocsURL: "https://geminicli.com/docs/hooks/reference/",
		Working: "BeforeAgent", Waiting: "Notification", Idle: "AfterAgent",
		Notes: "settings.json hooks; lifecycle matchers are exact strings",
	},
	{
		Tool: "cursor", ConfigPath: ".cursor/hooks.json",
		DocsURL: "https://cursor.com/docs/hooks",
		Working: "beforeSubmitPrompt", Waiting: "beforeShellExecution", Idle: "stop",
		Notes: "CLI hooks since v1.7 (sessionStart/stop/prompt)",
	},
	{
		Tool: "copilot", ConfigPath: "~/.copilot/hooks/notification-hooks.json",
		DocsURL: "https://docs.github.com/en/copilot/reference/hooks-reference",
		Working: "userPromptSubmit", Waiting: "(see docs)", Idle: "sessionEnd",
		Notes: "bash/powershell keys per OS",
	},
	{
		Tool: "codex", ConfigPath: "~/.codex/config.toml  (notify = [...])",
		DocsURL: "https://developers.openai.com/codex/config-advanced",
		Working: "—", Waiting: "approval-requested", Idle: "agent-turn-complete",
		Notes: "one notify program for all events; external notify currently fires turn-complete reliably",
	},
}

func hookByTool(tool string) (providerHook, bool) {
	for _, p := range providerHooks {
		if p.Tool == tool {
			return p, true
		}
	}
	return providerHook{}, false
}

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Install AI tool hooks so `sloop ps` knows agent status precisely",
}

var hooksInstallCmd = &cobra.Command{
	Use:       "install [tool]",
	Short:     "Install sloop status hooks (auto for claude; others: see `hooks print`)",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: hookTools(),
	RunE: func(cmd *cobra.Command, args []string) error {
		tool := "claude"
		if len(args) == 1 {
			tool = args[0]
		}
		p, ok := hookByTool(tool)
		if !ok {
			return fmt.Errorf("unknown tool %q (have: %s)", tool, strings.Join(hookTools(), ", "))
		}
		if !p.AutoInstall {
			cmd.Printf("auto-install isn't available for %s yet.\n", tool)
			cmd.Printf("Run `sloop hooks print %s` for the exact events to wire in %s.\n", tool, p.ConfigPath)
			return nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		ws, err := workspace.Resolve(cwd)
		if err != nil {
			return err
		}
		path, changed, err := installClaudeHooks(ws.Root)
		if err != nil {
			return err
		}
		if changed {
			cmd.Printf("installed sloop status hooks → %s\n", path)
		} else {
			cmd.Printf("sloop status hooks already present in %s\n", path)
		}
		cmd.Println("Claude will now report waiting/working/idle to `sloop ps`.")
		return nil
	},
}

var hooksPrintCmd = &cobra.Command{
	Use:       "print [tool]",
	Short:     "Print how to wire a tool's hooks to sloop (default: claude JSON snippet)",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: hookTools(),
	RunE: func(cmd *cobra.Command, args []string) error {
		tool := "claude"
		if len(args) == 1 {
			tool = args[0]
		}
		p, ok := hookByTool(tool)
		if !ok {
			return fmt.Errorf("unknown tool %q (have: %s)", tool, strings.Join(hookTools(), ", "))
		}
		if tool == "claude" {
			doc, _ := mergeClaudeHooks(nil)
			out, _ := json.MarshalIndent(doc, "", "  ")
			cmd.Printf("# %s — add to %s\n%s\n", tool, p.ConfigPath, string(out))
			return nil
		}
		cmd.Printf("# %s hooks → call these from %s\n", tool, p.ConfigPath)
		cmd.Printf("  working : on %-22s → run: %s\n", p.Working, hookCommandFor("working"))
		cmd.Printf("  waiting : on %-22s → run: %s\n", p.Waiting, hookCommandFor("waiting"))
		cmd.Printf("  idle    : on %-22s → run: %s\n", p.Idle, hookCommandFor("idle"))
		if p.Notes != "" {
			cmd.Printf("  note    : %s\n", p.Notes)
		}
		cmd.Printf("  docs    : %s\n", p.DocsURL)
		return nil
	},
}

var hooksListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show hook support and event mapping for every AI provider",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("%-9s %-12s %s\n", "TOOL", "AUTO-INSTALL", "CONFIG")
		for _, p := range providerHooks {
			auto := "print+paste"
			if p.AutoInstall {
				auto = "yes"
			}
			cmd.Printf("%-9s %-12s %s\n", p.Tool, auto, p.ConfigPath)
		}
		cmd.Println("\nDetails: sloop hooks print <tool>")
		return nil
	},
}

func hookTools() []string {
	out := make([]string, 0, len(providerHooks))
	for _, p := range providerHooks {
		out = append(out, p.Tool)
	}
	return out
}

func RegisterHooks(cmd *cobra.Command) {
	hooksCmd.AddCommand(hooksInstallCmd)
	hooksCmd.AddCommand(hooksPrintCmd)
	hooksCmd.AddCommand(hooksListCmd)
	cmd.AddCommand(hooksCmd)
}
