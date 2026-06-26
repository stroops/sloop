package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/config"
)

func TestRunInitScaffolds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir()) // isolate the global DB
	t.Setenv("PATH", t.TempDir()) // empty PATH: no tools detected

	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	for _, p := range []string{
		".sloop/config.yaml",
		".sloop/context/project.md",
		".sloop/profiles/claude.yaml",
		".sloop/.gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
	}
	for _, d := range []string{".sloop/skills", ".sloop/vault"} {
		if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
			t.Fatalf("expected dir %s: %v", d, err)
		}
	}
}

func TestRunInitFallsBackToClaude(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // empty PATH: no tools detected
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sloop", "profiles", "claude.yaml")); err != nil {
		t.Fatalf("expected claude fallback profile: %v", err)
	}
	p, err := config.LoadProject(filepath.Join(dir, ".sloop"))
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if p.DefaultTool != "claude" {
		t.Fatalf("want default claude, got %q", p.DefaultTool)
	}
}
