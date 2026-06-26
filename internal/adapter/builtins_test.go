package adapter

import "testing"

func TestLoadBuiltinHasAllTools(t *testing.T) {
	m, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}
	want := map[string]struct {
		launch string
		output string
	}{
		"claude":  {"claude", "CLAUDE.md"},
		"cursor":  {"agent", "AGENTS.md"},
		"codex":   {"codex", "AGENTS.md"},
		"copilot": {"copilot", "AGENTS.md"},
		"gemini":  {"gemini", "GEMINI.md"},
	}
	for key, exp := range want {
		got, ok := m[key]
		if !ok {
			t.Fatalf("built-in %q missing; have %v", key, keys(m))
		}
		if got.Launch != exp.launch {
			t.Errorf("%s launch = %q, want %q", key, got.Launch, exp.launch)
		}
		if len(got.Outputs) != 1 || got.Outputs[0].Path != exp.output {
			t.Errorf("%s outputs = %+v, want path %q", key, got.Outputs, exp.output)
		}
	}
}
