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
	if len(claude.Outputs) != 1 || claude.Outputs[0].Path != "CLAUDE.md" {
		t.Fatalf("unexpected outputs: %+v", claude.Outputs)
	}
}

func TestRenderDefaultTemplate(t *testing.T) {
	m := Manifest{Outputs: []Output{{Path: "CLAUDE.md", Template: "default"}}}
	out := m.Render("hello context")
	if out["CLAUDE.md"] != "hello context" {
		t.Fatalf("want passthrough, got %q", out["CLAUDE.md"])
	}
}

func keys(m map[string]Manifest) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}
