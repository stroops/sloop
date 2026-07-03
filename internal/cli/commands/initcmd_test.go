package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
)

func TestResolveInitToolsFlag(t *testing.T) {
	manifests := map[string]adapter.Manifest{
		"claude": {Name: "Claude Code"},
		"cursor": {Name: "Cursor CLI"},
	}
	defer func(v string) { initTools = v }(initTools)
	initTools = "claude, bogus, cursor, claude" // unknown dropped, dupes deduped

	got := resolveInitTools(&cobra.Command{}, manifests, nil, Interaction{})
	if len(got) != 2 || got[0] != "claude" || got[1] != "cursor" {
		t.Fatalf("resolveInitTools = %v, want [claude cursor]", got)
	}
}

func TestPrimaryFirst(t *testing.T) {
	got := primaryFirst([]string{"agy", "claude", "cursor"}, "claude")
	if len(got) != 3 || got[0] != "claude" {
		t.Fatalf("primaryFirst = %v, want claude first", got)
	}
	// Primary not in the list → order unchanged.
	if got := primaryFirst([]string{"cursor", "gemini"}, "claude"); got[0] != "cursor" {
		t.Fatalf("primaryFirst absent = %v", got)
	}
}

func TestHooksNeeded(t *testing.T) {
	root := t.TempDir()
	manifests := map[string]adapter.Manifest{
		"claude": {Hooks: adapter.HooksSpec{Install: "settings-json", Config: ".claude/settings.local.json", Events: adapter.HookEvents{Idle: adapter.EventSpec{Event: "Stop"}}}},
		"codex":  {Hooks: adapter.HooksSpec{Install: ""}}, // manual install → never "needed"
	}
	if !hooksNeeded(root, []string{"claude"}, manifests) {
		t.Fatal("claude hooks not installed → needed")
	}
	if hooksNeeded(root, []string{"codex"}, manifests) {
		t.Fatal("manual-install tool → not auto-needed")
	}
}

func TestStatuslineNeeded(t *testing.T) {
	root := t.TempDir()
	manifests := map[string]adapter.Manifest{
		"claude": {StatusLine: adapter.StatusLineSpec{Install: "settings-json", Config: filepath.Join(root, "settings.json")}},
		"codex":  {StatusLine: adapter.StatusLineSpec{Install: ""}}, // no statusline mechanism → never "needed"
	}
	if !statuslineNeeded(root, []string{"claude"}, manifests) {
		t.Fatal("claude statusline feed not installed → needed")
	}
	if statuslineNeeded(root, []string{"codex"}, manifests) {
		t.Fatal("no-statusline tool → not auto-needed")
	}
}

// installStatuslineForEnabled is init's counterpart to installHooksForEnabled:
// it should wire the statusline feed for every enabled settings-json tool
// (model + context % in the status bar) and be idempotent on a second run.
func TestInstallStatuslineForEnabled(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	// Isolate from a real second-account override in the ambient environment
	// (e.g. this test suite running inside a CLAUDE_CONFIG_DIR-set session),
	// which would redirect installs away from the sandboxed HOME above.
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	if _, err := RunInit(dir, []string{"claude"}, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	log := installStatuslineForEnabled(dir)
	if len(log) != 1 || !strings.Contains(log[0], "claude") {
		t.Fatalf("installStatuslineForEnabled = %v", log)
	}
	b, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}
	if !strings.Contains(string(b), "statusline feed claude") {
		t.Fatalf("settings.json missing feed command: %s", b)
	}

	if log2 := installStatuslineForEnabled(dir); len(log2) != 0 {
		t.Fatalf("second install must be a no-op, got %v", log2)
	}
}

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
