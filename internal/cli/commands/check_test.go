package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
)

// labelOf finds the check item whose label contains substr.
func findItem(items []checkItem, substr string) (checkItem, bool) {
	for _, it := range items {
		if strings.Contains(it.Label, substr) {
			return it, true
		}
	}
	return checkItem{}, false
}

func TestReadinessChecklist(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(sloopDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifests := map[string]adapter.Manifest{
		"claude": {
			Name:    "Claude Code",
			Context: adapter.ContextSpec{Mode: "pointer", File: "CLAUDE.md"},
			Hooks:   adapter.HooksSpec{Install: "settings-json", Config: ".claude/settings.local.json", Events: adapter.HookEvents{Idle: "Stop"}},
		},
	}
	proj := &config.Project{Tools: []string{"claude"}, DefaultTool: "claude"}

	// Bare workspace: no AGENTS.md, no pointer, no hooks → those should be gaps.
	items := readinessChecklist(root, sloopDir, proj, manifests)

	if it, ok := findItem(items, "AGENTS.md"); !ok || it.OK {
		t.Fatalf("AGENTS.md should be a gap: %+v", it)
	}
	if it, ok := findItem(items, "Default tool"); !ok || !it.OK {
		t.Fatalf("default tool should pass: %+v", it)
	}
	if it, ok := findItem(items, "context pointer not delivered"); !ok || it.OK || it.Fix != "sloop sync" {
		t.Fatalf("missing pointer should suggest sloop sync: %+v", it)
	}
	if it, ok := findItem(items, "status hooks not installed"); !ok || it.OK || !strings.Contains(it.Fix, "hooks install") {
		t.Fatalf("missing hooks should suggest install: %+v", it)
	}

	// Now satisfy AGENTS.md → that check flips to OK.
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items = readinessChecklist(root, sloopDir, proj, manifests)
	if it, ok := findItem(items, "AGENTS.md present"); !ok || !it.OK {
		t.Fatalf("AGENTS.md should now pass: %+v", it)
	}
}

func TestEvalReadiness(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "adir"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "afile"), []byte("x"), 0o644)

	cases := []struct {
		kind, path string
		want       bool
	}{
		{"file-exists", "afile", true},
		{"file-exists", "adir", false}, // a dir is not a file
		{"file-exists", "nope", false},
		{"dir-exists", "adir", true},
		{"dir-exists", "afile", false},
		{"weird", "afile", false}, // unknown kind
	}
	for _, c := range cases {
		got := evalReadiness(root, adapter.ReadinessCheck{Kind: c.kind, Path: c.path})
		if got != c.want {
			t.Errorf("evalReadiness(%s,%s) = %v, want %v", c.kind, c.path, got, c.want)
		}
	}
}

func TestReadinessManifestChecks(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	_ = os.MkdirAll(sloopDir, 0o755)
	_ = os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# g\n"), 0o644)

	// Pretend we're in a git repo where AGENTS.md is NOT committed.
	defer func(a func(string) bool, b func(string, string) bool) { inGitRepo, gitTracked = a, b }(inGitRepo, gitTracked)
	inGitRepo = func(string) bool { return true }
	gitTracked = func(_, _ string) bool { return false }

	manifests := map[string]adapter.Manifest{
		"claude": {
			Name:    "Claude Code",
			Context: adapter.ContextSpec{Mode: "native"},
			Readiness: adapter.ReadinessSpec{
				Docs: "https://example/best-practices",
				Checks: []adapter.ReadinessCheck{
					{ID: "req", Label: "Required dir", Kind: "dir-exists", Path: ".claude/agents", Fix: "make it"},
					{ID: "opt", Label: "Optional dir", Kind: "dir-exists", Path: ".claude/extra", Optional: true},
				},
			},
		},
	}
	proj := &config.Project{Tools: []string{"claude"}, DefaultTool: "claude"}
	items := readinessChecklist(root, sloopDir, proj, manifests)

	// AGENTS.md exists but not committed → that check is a gap.
	if it, ok := findItem(items, "committed to git"); !ok || it.OK {
		t.Fatalf("uncommitted AGENTS.md should be a gap: %+v", it)
	}
	// Required manifest check missing → gap with its fix.
	if it, ok := findItem(items, "Required dir"); !ok || it.OK || it.Fix != "make it" {
		t.Fatalf("required readiness check should be a gap with fix: %+v", it)
	}
	// Optional manifest check missing → advisory info, not a gap.
	if it, ok := findItem(items, "Optional dir"); !ok || it.OK || !it.Info {
		t.Fatalf("optional readiness check should be info, not a gap: %+v", it)
	}
}
