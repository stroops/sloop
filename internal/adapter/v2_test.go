package adapter

import "testing"

func TestManifestV2Fields(t *testing.T) {
	m, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}
	if m["claude"].Context.Mode != "pointer" || m["claude"].Context.File != "CLAUDE.md" {
		t.Fatalf("claude context = %+v", m["claude"].Context)
	}
	if m["claude"].Skills.Target != ".claude/skills" {
		t.Fatalf("claude skills = %+v", m["claude"].Skills)
	}
	if m["gemini"].Context.Mode != "pointer" || m["gemini"].Context.File != "GEMINI.md" {
		t.Fatalf("gemini context = %+v", m["gemini"].Context)
	}
	for _, native := range []string{"cursor", "codex", "copilot"} {
		if m[native].Context.Mode != "native" {
			t.Fatalf("%s should be native, got %q", native, m[native].Context.Mode)
		}
	}
}
