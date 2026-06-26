package adapter

import "testing"

func TestLoadBuiltinHasAllTools(t *testing.T) {
	m, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}
	want := map[string]struct {
		launch string
	}{
		"claude":  {"claude"},
		"cursor":  {"agent"},
		"codex":   {"codex"},
		"copilot": {"copilot"},
		"gemini":  {"gemini"},
	}
	for key, exp := range want {
		got, ok := m[key]
		if !ok {
			t.Fatalf("built-in %q missing; have %v", key, keys(m))
		}
		if got.Launch != exp.launch {
			t.Errorf("%s launch = %q, want %q", key, got.Launch, exp.launch)
		}
	}
}
