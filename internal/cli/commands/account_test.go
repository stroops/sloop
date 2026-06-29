package commands

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

func accountManifests() map[string]adapter.Manifest {
	return map[string]adapter.Manifest{
		"claude": {Name: "Claude", Detect: "claude", Launch: "claude", Account: adapter.AccountSpec{
			ConfigDirEnv: "CLAUDE_CONFIG_DIR",
			DefaultDir:   "~/.claude",
			Share:        []string{"plugins", "CLAUDE.md"},
			ShareState:   []string{"projects"},
		}},
		"cursor": {Name: "Cursor", Detect: "cursor", Launch: "cursor"},
	}
}

func TestResolveAccountTool(t *testing.T) {
	m := accountManifests()

	// Inferred: claude is the only tool declaring an account config dir.
	key, spec, err := resolveAccountTool("", m)
	if err != nil || key != "claude" || spec.ConfigDirEnv != "CLAUDE_CONFIG_DIR" {
		t.Fatalf("infer: key=%q spec=%+v err=%v", key, spec, err)
	}

	// Explicit tool that has no account support is rejected.
	if _, _, err := resolveAccountTool("cursor", m); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("want not-supported error, got %v", err)
	}

	// Unknown tool.
	if _, _, err := resolveAccountTool("nope", m); err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("want unknown-tool error, got %v", err)
	}

	// Ambiguous: two tools declare it → must pick.
	two := accountManifests()
	c := two["cursor"]
	c.Account = adapter.AccountSpec{ConfigDirEnv: "CURSOR_CONFIG_DIR"}
	two["cursor"] = c
	if _, _, err := resolveAccountTool("", two); err == nil || !strings.Contains(err.Error(), "several tools") {
		t.Fatalf("want ambiguity error, got %v", err)
	}
}

func TestLinkShared(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mustMkdir(t, filepath.Join(src, "plugins"))
	mustWrite(t, filepath.Join(src, "CLAUDE.md"), "hi")
	mustWrite(t, filepath.Join(src, credentialsFile), "secret")
	// Pre-existing target must not be clobbered.
	mustWrite(t, filepath.Join(dst, "CLAUDE.md"), "mine")

	var out strings.Builder
	linkShared([]string{"plugins", "CLAUDE.md", credentialsFile, "missing"}, src, dst, &out)

	// plugins linked
	if target, err := os.Readlink(filepath.Join(dst, "plugins")); err != nil || target != filepath.Join(src, "plugins") {
		t.Fatalf("plugins not linked: %v %q", err, target)
	}
	// CLAUDE.md left as the user's own file (not a symlink)
	if fi, err := os.Lstat(filepath.Join(dst, "CLAUDE.md")); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("existing CLAUDE.md should be untouched: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "CLAUDE.md")); string(b) != "mine" {
		t.Fatalf("CLAUDE.md content changed: %q", b)
	}
	// credentials must never be linked
	if _, err := os.Lstat(filepath.Join(dst, credentialsFile)); !os.IsNotExist(err) {
		t.Fatalf("credentials must not be shared (err=%v)", err)
	}
	// missing source produces no link
	if _, err := os.Lstat(filepath.Join(dst, "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing source should be skipped (err=%v)", err)
	}
}

func TestSetupAccountDirCreatesAndShares(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(home, ".claude")
	mustMkdir(t, filepath.Join(src, "plugins"))
	mustWrite(t, filepath.Join(src, "projects"), "") // a file is enough to be sharable
	dst := filepath.Join(home, ".claude-work")

	spec := adapter.AccountSpec{DefaultDir: src, Share: []string{"plugins"}, ShareState: []string{"projects"}}

	// Auto: creates the dir, shares tooling, but never history (interactive=false).
	var out strings.Builder
	setupAccountDir(Interaction{Auto: true}, spec, dst, false, bufio.NewReader(strings.NewReader("")), &out)

	if fi, err := os.Stat(dst); err != nil || !fi.IsDir() {
		t.Fatalf("dst not created: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "plugins")); err != nil {
		t.Fatalf("tooling not shared under auto: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "projects")); !os.IsNotExist(err) {
		t.Fatalf("history must not be shared without an interactive yes (err=%v)", err)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
