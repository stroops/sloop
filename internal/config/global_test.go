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
