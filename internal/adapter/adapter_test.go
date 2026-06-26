package adapter

import "testing"

func TestLoadBuiltinClaude(t *testing.T) {
	m, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}
	claude, ok := m["claude"]
	if !ok {
		t.Fatalf("claude adapter missing; got keys %v", keys(m))
	}
	if claude.Launch != "claude" || claude.Detect != "claude" {
		t.Fatalf("unexpected manifest: %+v", claude)
	}
}

func keys(m map[string]Manifest) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}
