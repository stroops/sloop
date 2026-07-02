package adapter

import "testing"

func keys(m map[string]Manifest) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}

// TestBuiltinManifests asserts the shipped adapter set: presence, launch
// binaries, context modes, skills targets, and which tools auto-install hooks.
// (Consolidates the former adapter_test/builtins_test/v2_test.)
func TestBuiltinManifests(t *testing.T) {
	m, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}

	want := map[string]struct {
		launch  string
		mode    string
		skills  string
		install string
	}{
		"claude":  {"claude", "pointer", ".claude/skills", "settings-json"},
		"gemini":  {"gemini", "pointer", "", "settings-json"},
		"cursor":  {"agent", "native", "", "cursor-json"},
		"codex":   {"codex", "native", "", "codex-toml"},
		"copilot": {"copilot", "native", "", "copilot-json"},
		"agy":     {"agy", "native", "", ""},
	}
	for key, exp := range want {
		got, ok := m[key]
		if !ok {
			t.Fatalf("built-in %q missing; have %v", key, keys(m))
		}
		if got.Launch != exp.launch {
			t.Errorf("%s launch = %q, want %q", key, got.Launch, exp.launch)
		}
		if got.Context.Mode != exp.mode {
			t.Errorf("%s context mode = %q, want %q", key, got.Context.Mode, exp.mode)
		}
		if got.Skills.Target != exp.skills {
			t.Errorf("%s skills target = %q, want %q", key, got.Skills.Target, exp.skills)
		}
		if got.Hooks.Install != exp.install {
			t.Errorf("%s hooks install = %q, want %q", key, got.Hooks.Install, exp.install)
		}
	}
}

func TestBuiltinEventSpecs(t *testing.T) {
	ms, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	claude := ms["claude"]
	if claude.Hooks.Events.Working.Event != "UserPromptSubmit" {
		t.Fatalf("claude working = %+v", claude.Hooks.Events.Working)
	}
	cursor := ms["cursor"]
	if cursor.Hooks.Events.Waiting.Event != "" {
		t.Fatalf("cursor waiting should stay unset, got %+v", cursor.Hooks.Events.Waiting)
	}
	copilot := ms["copilot"]
	if copilot.Hooks.Install != "copilot-json" || copilot.Hooks.Events.Waiting.Matcher != "permission_prompt" {
		t.Fatalf("copilot hooks = %+v", copilot.Hooks)
	}
}
