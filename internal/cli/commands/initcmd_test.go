package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/config"
)

func TestRunInitScaffolds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir()) // isolate the global DB
	t.Setenv("PATH", t.TempDir()) // empty PATH: no tools detected

	if err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sloop", "context")); !os.IsNotExist(err) {
		t.Fatalf(".sloop/context should not exist")
	}

	for _, p := range []string{
		".sloop/config.yaml",
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
	if err := RunInit(dir, false); err != nil {
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

func TestRunInitScanPopulatesAgents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunInit(dir, true); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "go test ./...") {
		t.Fatalf("scanned AGENTS.md should contain detected commands:\n%s", string(b))
	}
}
