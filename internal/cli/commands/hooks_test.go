package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// claudeEvents mirrors the claude manifest's event→command mapping.
var claudeEvents = map[string]string{
	"UserPromptSubmit": "sloop hooks emit working",
	"Notification":     "sloop hooks emit waiting",
	"Stop":             "sloop hooks emit idle",
}

// geminiEvents mirrors the gemini manifest's mapping (different event names,
// same settings.json shape); proves the installer is multi-provider.
var geminiEvents = map[string]string{
	"BeforeAgent":  "sloop hooks emit working",
	"Notification": "sloop hooks emit waiting",
	"AfterAgent":   "sloop hooks emit idle",
}

func TestMergeSettingsHooksIdempotentAndPreserving(t *testing.T) {
	root := map[string]any{
		"model": "opus",
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{"hooks": []any{
					map[string]any{"type": "command", "command": "echo keepme"},
				}},
			},
		},
	}
	merged, changed := mergeSettingsHooks(root, claudeEvents)
	if !changed {
		t.Fatal("expected first merge to change")
	}
	if merged["model"] != "opus" {
		t.Fatal("existing key dropped")
	}
	hooks := merged["hooks"].(map[string]any)
	for event, cmd := range claudeEvents {
		if !hasCommandHook(hooks[event], cmd) {
			t.Fatalf("missing hook for %s", event)
		}
	}
	if !hasCommandHook(hooks["Stop"], "echo keepme") {
		t.Fatal("foreign Stop hook clobbered")
	}
	if _, changed := mergeSettingsHooks(merged, claudeEvents); changed {
		t.Fatal("second merge should not change")
	}
}

func TestInstallSettingsHooksGemini(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gemini", "settings.json")

	changed, err := installSettingsHooks(path, geminiEvents)
	if err != nil || !changed {
		t.Fatalf("install: changed=%v err=%v", changed, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("written file not valid JSON: %v", err)
	}
	hooks := doc["hooks"].(map[string]any)
	if !hasCommandHook(hooks["Notification"], "sloop hooks emit waiting") {
		t.Fatal("gemini Notification hook missing")
	}
	// Re-install is idempotent.
	if changed, _ := installSettingsHooks(path, geminiEvents); changed {
		t.Fatal("re-install should not change")
	}
}

// cursorEvents mirrors the cursor manifest's mapping (no waiting event; flat
// hooks.json shape).
var cursorEvents = map[string]string{
	"beforeSubmitPrompt": "sloop hooks emit working",
	"stop":               "sloop hooks emit idle",
}

func TestMergeCursorHooksIdempotentAndPreserving(t *testing.T) {
	root := map[string]any{
		"version": float64(1),
		"hooks": map[string]any{
			"stop": []any{map[string]any{"command": "./hooks/audit.sh"}},
		},
	}
	merged, changed := mergeCursorHooks(root, cursorEvents)
	if !changed {
		t.Fatal("expected first merge to change")
	}
	hooks := merged["hooks"].(map[string]any)
	for event, cmd := range cursorEvents {
		if !hasCursorCommand(hooks[event], cmd) {
			t.Fatalf("missing hook for %s", event)
		}
	}
	// Foreign stop hook is preserved alongside sloop's.
	if !hasCursorCommand(hooks["stop"], "./hooks/audit.sh") {
		t.Fatal("foreign stop hook clobbered")
	}
	if _, changed := mergeCursorHooks(merged, cursorEvents); changed {
		t.Fatal("second merge should not change")
	}
}

func TestInstallCursorHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor", "hooks.json")

	changed, err := installCursorHooks(path, cursorEvents)
	if err != nil || !changed {
		t.Fatalf("install: changed=%v err=%v", changed, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("written file not valid JSON: %v", err)
	}
	if doc["version"] != float64(1) {
		t.Fatalf("version = %v, want 1", doc["version"])
	}
	hooks := doc["hooks"].(map[string]any)
	if !hasCursorCommand(hooks["beforeSubmitPrompt"], "sloop hooks emit working") {
		t.Fatal("cursor beforeSubmitPrompt hook missing")
	}
	if changed, _ := installCursorHooks(path, cursorEvents); changed {
		t.Fatal("re-install should not change")
	}
}

func TestHookInstallerDispatch(t *testing.T) {
	if hookInstaller("settings-json") == nil || hookInstaller("cursor-json") == nil {
		t.Fatal("known strategies must have an installer")
	}
	if hookInstaller("") != nil || hookInstaller("codex-toml") != nil {
		t.Fatal("unknown/manual strategies must have no installer")
	}
}

func TestResolveHookConfigPath(t *testing.T) {
	if p, _ := resolveHookConfigPath("/repo", ".claude/settings.local.json"); p != "/repo/.claude/settings.local.json" {
		t.Fatalf("repo-relative = %q", p)
	}
	home, _ := os.UserHomeDir()
	if p, _ := resolveHookConfigPath("/repo", "~/.codex/config.toml"); p != filepath.Join(home, ".codex/config.toml") {
		t.Fatalf("home-relative = %q", p)
	}
}
