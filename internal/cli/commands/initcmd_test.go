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
	t.Setenv("PATH", t.TempDir()) // empty PATH: no tools detected → minimal claude-only

	if _, err := RunInit(dir, nil, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	// A lone pointer-mode tool stays minimal: no AGENTS.md / pointer (the tool
	// keeps its own context). sloop just sets up .sloop and registers.
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("single-tool init should NOT create AGENTS.md")
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("single-tool init should NOT create a CLAUDE.md pointer")
	}

	for _, p := range []string{".sloop/config.yaml", ".sloop/.gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
	}
	for _, d := range []string{".sloop/skills", ".sloop/vault"} {
		if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
			t.Fatalf("expected dir %s: %v", d, err)
		}
	}
	// Profiles were removed: init must NOT scaffold per-tool profile files.
	if _, err := os.Stat(filepath.Join(dir, ".sloop", "profiles")); !os.IsNotExist(err) {
		t.Fatalf(".sloop/profiles should not be created")
	}
}

func TestRunInitCanonicalDelivers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	// Two tools → canonical workspace: AGENTS.md + the CLAUDE.md pointer appear.
	if _, err := RunInit(dir, []string{"claude", "cursor"}, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md in a canonical workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md pointer delivered: %v", err)
	}
}

func TestRunInitFallsBackToClaude(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // empty PATH: no tools detected
	if _, err := RunInit(dir, nil, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	p, err := config.LoadProject(filepath.Join(dir, ".sloop"))
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if p.DefaultTool != "claude" {
		t.Fatalf("want default claude, got %q", p.DefaultTool)
	}
	if !contains(p.Tools, "claude") {
		t.Fatalf("want claude enabled, got %v", p.Tools)
	}
}

func TestRunInitScaffoldCreatesStandardFolders(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no tools → claude fallback
	initScaffold = true
	defer func() { initScaffold = false }()

	if _, err := RunInit(dir, nil, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	for _, d := range []string{".claude/skills", ".claude/agents"} {
		if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
			t.Fatalf("expected scaffolded dir %s: %v", d, err)
		}
	}
}

func TestRunInitScanPopulatesAgents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Scan pre-fills AGENTS.md, which only exists in a canonical workspace.
	if _, err := RunInit(dir, []string{"claude", "cursor"}, true); err != nil {
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
