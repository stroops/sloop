package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIncludesBuiltinCursor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := m["claude"]; !ok {
		t.Fatalf("claude built-in missing")
	}
	cursor, ok := m["cursor"]
	if !ok {
		t.Fatalf("cursor built-in missing")
	}
	if cursor.Launch != "agent" {
		t.Fatalf("unexpected cursor manifest: %+v", cursor)
	}
}

func TestLoadUserAdapterOverridesBuiltin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	adaptersDir := filepath.Join(home, ".sloop", "adapters")
	if err := os.MkdirAll(adaptersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Override claude's launch command via a user manifest.
	custom := "name: My Claude\ndetect: claude\nlaunch: claude-custom\n"
	if err := os.WriteFile(filepath.Join(adaptersDir, "claude.yaml"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m["claude"].Launch != "claude-custom" {
		t.Fatalf("user override not applied: %+v", m["claude"])
	}
}
