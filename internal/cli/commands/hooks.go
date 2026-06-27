package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/workspace"
)

// Hook installation is driven entirely by adapter manifests (the single
// provider-aware source). A tool's manifest says where its hook config lives,
// which install strategy applies, and which of its events map to sloop's
// working/waiting/idle states. Each event calls `sloop hook <state>`, which
// records a marker `sloop ps` reads.

// hookCommandFor is the shell command a tool runs for a sloop state.
func hookCommandFor(state string) string { return "sloop hook " + state }

// eventCommands turns a manifest's event→state mapping into the event→command
// map the installers and printers use, skipping states the tool can't signal.
func eventCommands(h adapter.HooksSpec) map[string]string {
	m := map[string]string{}
	add := func(event, state string) {
		if event != "" {
			m[event] = hookCommandFor(state)
		}
	}
	add(h.Events.Working, "working")
	add(h.Events.Waiting, "waiting")
	add(h.Events.Idle, "idle")
	return m
}

// mergeSettingsHooks adds the given event→command hooks to a settings.json-style
// document (the shape Claude and Gemini share) without disturbing existing keys
// or other hooks. Returns the (possibly unchanged) document and whether it changed.
func mergeSettingsHooks(root map[string]any, events map[string]string) (map[string]any, bool) {
	if root == nil {
		root = map[string]any{}
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	changed := false
	for event, cmd := range events {
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

// installSettingsHooks merges events into the JSON settings file at path,
// creating it if needed. Returns whether the file changed.
func installSettingsHooks(path string, events map[string]string) (bool, error) {
	doc := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &doc); err != nil {
			return false, fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}
	merged, changed := mergeSettingsHooks(doc, events)
	if !changed {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644)
}

// resolveHookConfigPath turns a manifest config path into an absolute path: ~/…
// expands to the home dir, absolute paths pass through, and a repo-relative path
// is joined to the workspace root.
func resolveHookConfigPath(root, cfg string) (string, error) {
	if strings.HasPrefix(cfg, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, cfg[2:]), nil
	}
	if filepath.IsAbs(cfg) {
		return cfg, nil
	}
	return filepath.Join(root, cfg), nil
}

// hookTools returns the adapter keys, sorted, for completion and listing.
func hookTools() []string {
	manifests, err := adapter.Load()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(manifests))
	for k := range manifests {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func manifestForTool(tool string) (adapter.Manifest, error) {
	manifests, err := adapter.Load()
	if err != nil {
		return adapter.Manifest{}, err
	}
	m, ok := manifests[tool]
	if !ok {
		return adapter.Manifest{}, fmt.Errorf("unknown tool %q (have: %s)", tool, strings.Join(hookTools(), ", "))
	}
	return m, nil
}

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Install AI tool hooks so `sloop ps` knows agent status precisely",
}

var hooksInstallCmd = &cobra.Command{
	Use:   "install [tool]",
	Short: "Install sloop status hooks (auto for settings-json tools; others: see `hooks print`)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tool := "claude"
		if len(args) == 1 {
			tool = args[0]
		}
		m, err := manifestForTool(tool)
		if err != nil {
			return err
		}
		if m.Hooks.Install != "settings-json" {
			cmd.Printf("auto-install isn't available for %s yet.\n", tool)
			cmd.Printf("Run `sloop hooks print %s` for the exact events to wire in %s.\n", tool, m.Hooks.Config)
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
		path, err := resolveHookConfigPath(ws.Root, m.Hooks.Config)
		if err != nil {
			return err
		}
		changed, err := installSettingsHooks(path, eventCommands(m.Hooks))
		if err != nil {
			return err
		}
		if changed {
			cmd.Printf("installed sloop status hooks → %s\n", path)
		} else {
			cmd.Printf("sloop status hooks already present in %s\n", path)
		}
		cmd.Printf("%s will now report waiting/working/idle to `sloop ps`.\n", m.Name)
		return nil
	},
}

var hooksPrintCmd = &cobra.Command{
	Use:   "print [tool]",
	Short: "Print how to wire a tool's hooks to sloop (settings-json tools print a JSON snippet)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tool := "claude"
		if len(args) == 1 {
			tool = args[0]
		}
		m, err := manifestForTool(tool)
		if err != nil {
			return err
		}
		if m.Hooks.Install == "settings-json" {
			doc, _ := mergeSettingsHooks(nil, eventCommands(m.Hooks))
			out, _ := json.MarshalIndent(doc, "", "  ")
			cmd.Printf("# %s — add to %s\n%s\n", tool, m.Hooks.Config, string(out))
			return nil
		}
		cmd.Printf("# %s hooks → call these from %s\n", tool, m.Hooks.Config)
		cmd.Printf("  working : on %-22s → run: %s\n", orDash(m.Hooks.Events.Working), hookCommandFor("working"))
		cmd.Printf("  waiting : on %-22s → run: %s\n", orDash(m.Hooks.Events.Waiting), hookCommandFor("waiting"))
		cmd.Printf("  idle    : on %-22s → run: %s\n", orDash(m.Hooks.Events.Idle), hookCommandFor("idle"))
		if m.Hooks.Notes != "" {
			cmd.Printf("  note    : %s\n", m.Hooks.Notes)
		}
		if m.Hooks.Docs != "" {
			cmd.Printf("  docs    : %s\n", m.Hooks.Docs)
		}
		return nil
	},
}

var hooksListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show hook support and event mapping for every AI provider",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		manifests, err := adapter.Load()
		if err != nil {
			return err
		}
		cmd.Printf("%-9s %-12s %s\n", "TOOL", "AUTO-INSTALL", "CONFIG")
		for _, tool := range hookTools() {
			m := manifests[tool]
			auto := "print+paste"
			if m.Hooks.Install == "settings-json" {
				auto = "yes"
			}
			cmd.Printf("%-9s %-12s %s\n", tool, auto, m.Hooks.Config)
		}
		cmd.Println("\nDetails: sloop hooks print <tool>")
		return nil
	},
}

func RegisterHooks(cmd *cobra.Command) {
	tools := hookTools()
	hooksInstallCmd.ValidArgs = tools
	hooksPrintCmd.ValidArgs = tools
	hooksCmd.AddCommand(hooksInstallCmd)
	hooksCmd.AddCommand(hooksPrintCmd)
	hooksCmd.AddCommand(hooksListCmd)
	cmd.AddCommand(hooksCmd)
}
