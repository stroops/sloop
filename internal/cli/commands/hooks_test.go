package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeClaudeHooksIdempotent(t *testing.T) {
	// Pre-existing unrelated content must be preserved.
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
	merged, changed := mergeClaudeHooks(root)
	if !changed {
		t.Fatal("expected first merge to change")
	}
	if merged["model"] != "opus" {
		t.Fatal("existing key dropped")
	}
	// All three sloop hooks present.
	hooks := merged["hooks"].(map[string]any)
	for event, state := range sloopHooks {
		if !hasCommandHook(hooks[event], hookCommandFor(state)) {
			t.Fatalf("missing sloop hook for %s", event)
		}
	}
	// Existing Stop hook preserved alongside sloop's.
	if !hasCommandHook(hooks["Stop"], "echo keepme") {
		t.Fatal("foreign Stop hook clobbered")
	}
	// Second merge is a no-op.
	if _, changed := mergeClaudeHooks(merged); changed {
		t.Fatal("second merge should not change")
	}
}

func TestInstallClaudeHooksWritesFile(t *testing.T) {
	dir := t.TempDir()
	path, changed, err := installClaudeHooks(dir)
	if err != nil || !changed {
		t.Fatalf("install: changed=%v err=%v", changed, err)
	}
	if path != filepath.Join(dir, ".claude", "settings.local.json") {
		t.Fatalf("path = %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("written file not valid JSON: %v", err)
	}
	// Re-install is idempotent.
	if _, changed, _ := installClaudeHooks(dir); changed {
		t.Fatal("re-install should not change")
	}
}
