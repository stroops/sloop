package config

import "testing"

func TestLoadGlobalDefaultsToAsk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	g, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if g.Mode != ModeAsk {
		t.Fatalf("want default mode %q, got %q", ModeAsk, g.Mode)
	}
}

func TestSaveAndLoadGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveGlobal(&Global{Mode: ModeAuto}); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	g, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if g.Mode != ModeAuto {
		t.Fatalf("want %q, got %q", ModeAuto, g.Mode)
	}
}

func TestGlobalProfilesRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	in := &Global{Profiles: map[string]Profile{
		"sec": {Tool: "claude", Env: map[string]string{"CLAUDE_CONFIG_DIR": "~/.claude-sec"}},
	}}
	if err := SaveGlobal(in); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	g, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	p, ok := g.Profiles["sec"]
	if !ok || p.Tool != "claude" || p.Env["CLAUDE_CONFIG_DIR"] != "~/.claude-sec" {
		t.Fatalf("profile did not round-trip: %+v", g.Profiles)
	}
}

func TestLoadGlobalNoProfilesIsNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveGlobal(&Global{Mode: ModeAuto}); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	g, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if g.Profiles != nil {
		t.Fatalf("want nil Profiles, got %+v", g.Profiles)
	}
}
