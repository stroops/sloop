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
	if it, ok := findItem(items, "AGENTS.md"); !ok || !it.OK {
		t.Fatalf("AGENTS.md should now pass: %+v", it)
	}
}
