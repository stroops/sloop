package commands

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
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
	if hookInstaller("settings-json") == nil || hookInstaller("cursor-json") == nil || hookInstaller("copilot-json") == nil || hookInstaller("codex-toml") == nil {
		t.Fatal("known strategies must have an installer")
	}
	if hookInstaller("") != nil {
		t.Fatal("unknown/manual strategies must have no installer")
	}
}

func TestResolveHookConfigPath(t *testing.T) {
	if p, _ := resolveHookConfigPath("/repo", adapter.AccountSpec{}, ".claude/settings.local.json"); p != "/repo/.claude/settings.local.json" {
		t.Fatalf("repo-relative = %q", p)
	}
	home, _ := os.UserHomeDir()
	if p, _ := resolveHookConfigPath("/repo", adapter.AccountSpec{}, "~/.codex/config.toml"); p != filepath.Join(home, ".codex/config.toml") {
		t.Fatalf("home-relative = %q", p)
	}
}

// TestResolveHookConfigPathConfigDirEnv proves a second-account profile
// (CLAUDE_CONFIG_DIR-style env var set) redirects the config path to that
// account's dir instead of the default one — the config_dir_env fix.
func TestResolveHookConfigPathConfigDirEnv(t *testing.T) {
	acct := adapter.AccountSpec{ConfigDirEnv: "CLAUDE_CONFIG_DIR", DefaultDir: "~/.claude"}
	t.Setenv("CLAUDE_CONFIG_DIR", "/work/claude-work")
	p, err := resolveHookConfigPath("/repo", acct, "~/.claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/work/claude-work/settings.json"; p != want {
		t.Fatalf("config_dir_env override = %q, want %q", p, want)
	}

	// Unset: falls back to the default ~/ expansion.
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, _ := os.UserHomeDir()
	if p, _ := resolveHookConfigPath("/repo", acct, "~/.claude/settings.json"); p != filepath.Join(home, ".claude/settings.json") {
		t.Fatalf("no env set = %q", p)
	}
}

func TestCopilotHooksDoc(t *testing.T) {
	h := adapter.HooksSpec{Events: adapter.HookEvents{
		Working: adapter.EventSpec{Event: "userPromptSubmitted"},
		Waiting: adapter.EventSpec{Event: "notification", Matcher: "permission_prompt"},
		Idle:    adapter.EventSpec{Event: "agentStop"},
	}}
	doc := copilotHooksDoc(h)
	if doc["version"] != 1 {
		t.Fatalf("version = %v", doc["version"])
	}
	hooks := doc["hooks"].(map[string]any)
	waiting := hooks["notification"].([]any)[0].(map[string]any)
	if waiting["command"] != "sloop hooks emit waiting" || waiting["matcher"] != "permission_prompt" {
		t.Fatalf("waiting entry = %v", waiting)
	}
	working := hooks["userPromptSubmitted"].([]any)[0].(map[string]any)
	if _, has := working["matcher"]; has {
		t.Fatal("matcher key must be absent when empty")
	}
}

func TestInstallCopilotHooksIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks", "sloop.json")
	h := adapter.HooksSpec{Events: adapter.HookEvents{
		Working: adapter.EventSpec{Event: "userPromptSubmitted"},
		Waiting: adapter.EventSpec{Event: "notification", Matcher: "permission_prompt"},
		Idle:    adapter.EventSpec{Event: "agentStop"},
	}}
	changed, err := installCopilotHooks(path, h)
	if err != nil || !changed {
		t.Fatalf("first install: changed=%v err=%v", changed, err)
	}
	b, _ := os.ReadFile(path)
	var doc map[string]any
	if json.Unmarshal(b, &doc) != nil {
		t.Fatal("wrote invalid JSON")
	}
	changed, err = installCopilotHooks(path, h)
	if err != nil || changed {
		t.Fatalf("second install must be a no-op: changed=%v err=%v", changed, err)
	}
}

func TestInstallCodexHooks(t *testing.T) {
	h := adapter.HooksSpec{}
	t.Run("empty file gets notify", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte("model = \"gpt-5.5\"\n\n[profiles.fast]\nmodel = \"gpt-5.5-mini\"\n"), 0o644)
		changed, err := installCodexHooks(path, h)
		if err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
		b, _ := os.ReadFile(path)
		s := string(b)
		if !strings.HasPrefix(s, "notify = [\"sloop\", \"hooks\", \"notify\", \"codex\"]\n") {
			t.Fatalf("notify not prepended:\n%s", s)
		}
		if !strings.Contains(s, "[profiles.fast]") {
			t.Fatal("existing content lost")
		}
		if changed, err := installCodexHooks(path, h); err != nil || changed {
			t.Fatalf("second run must no-op: changed=%v err=%v", changed, err)
		}
	})
	t.Run("missing file created", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		if changed, err := installCodexHooks(path, h); err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
	})
	t.Run("occupied slot untouched", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		orig := "notify = [\"terminal-notifier\"]\n"
		os.WriteFile(path, []byte(orig), 0o644)
		changed, err := installCodexHooks(path, h)
		if !errors.Is(err, errNotifyOccupied) || changed {
			t.Fatalf("want errNotifyOccupied, got changed=%v err=%v", changed, err)
		}
		if b, _ := os.ReadFile(path); string(b) != orig {
			t.Fatal("occupied config was modified")
		}
	})
	t.Run("unparseable file untouched", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte("not = = toml"), 0o644)
		if _, err := installCodexHooks(path, h); err == nil {
			t.Fatal("want parse error")
		}
	})
}

func TestNotifyStateFor(t *testing.T) {
	h := adapter.HooksSpec{Events: adapter.HookEvents{
		Waiting: adapter.EventSpec{Event: "approval-requested"},
		Idle:    adapter.EventSpec{Event: "agent-turn-complete"},
	}}
	cases := []struct{ payload, want string }{
		{`{"type":"agent-turn-complete","last-assistant-message":"hi"}`, "idle"},
		{`{"type":"approval-requested"}`, "waiting"},
		{`{"type":"something-new"}`, ""}, // unknown type → no state, no error
		{`not json`, ""},                 // malformed → no state, no error
		{``, ""},
	}
	for _, c := range cases {
		if got := notifyStateFor(h, []byte(c.payload)); got != c.want {
			t.Fatalf("payload %q: got %q want %q", c.payload, got, c.want)
		}
	}
}

func TestHooksInstalledForNewStrategies(t *testing.T) {
	ms, err := adapter.Load()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, tool := range []string{"copilot", "codex"} {
		m := ms[tool]
		if hooksInstalledFor(home, m) {
			t.Fatalf("%s: reported installed before install", tool)
		}
		path, err := resolveHookConfigPath(home, m.Account, m.Hooks.Config)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := hookInstaller(m.Hooks.Install)(path, m.Hooks); err != nil {
			t.Fatal(err)
		}
		if !hooksInstalledFor(home, m) {
			t.Fatalf("%s: not detected after install", tool)
		}
	}
}
