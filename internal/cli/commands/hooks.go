package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Install AI tool hooks so `sloop ps` knows agent status precisely",
}

var hooksInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install sloop status hooks into Claude (.claude/settings.local.json)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	Use:   "print",
	Short: "Print the Claude hooks JSON snippet (to install by hand)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		doc, _ := mergeClaudeHooks(nil)
		out, _ := json.MarshalIndent(doc, "", "  ")
		cmd.Println(string(out))
		return nil
	},
}

func RegisterHooks(cmd *cobra.Command) {
	hooksCmd.AddCommand(hooksInstallCmd)
	hooksCmd.AddCommand(hooksPrintCmd)
	cmd.AddCommand(hooksCmd)
}
