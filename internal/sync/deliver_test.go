package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

func TestEnsureAgentsCreatesThenSkips(t *testing.T) {
	root := t.TempDir()
	a, err := EnsureAgents(root)
	if err != nil || a != ActionCreated {
		t.Fatalf("first EnsureAgents = %v, %v", a, err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md not created: %v", err)
	}
	a, _ = EnsureAgents(root)
	if a != ActionSkipped {
		t.Fatalf("second EnsureAgents = %v, want skipped", a)
	}
}

func TestSyncContextPointerCreateIdempotentForeign(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Name: "Claude Code", Context: adapter.ContextSpec{Mode: "pointer", File: "CLAUDE.md"}}

	a, err := SyncContext(root, m)
	if err != nil || a != ActionCreated {
		t.Fatalf("create = %v, %v", a, err)
	}
	a, _ = SyncContext(root, m)
	if a != ActionSkipped {
		t.Fatalf("idempotent = %v, want skipped", a)
	}
	// User edits it → foreign, left untouched.
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("MY OWN CONTENT"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ = SyncContext(root, m)
	if a != ActionForeign {
		t.Fatalf("foreign = %v, want foreign", a)
	}
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if string(b) != "MY OWN CONTENT" {
		t.Fatalf("foreign file was overwritten: %q", string(b))
	}
}

func TestSyncContextNativeDoesNothing(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Context: adapter.ContextSpec{Mode: "native"}}
	a, err := SyncContext(root, m)
	if err != nil || a != ActionSkipped {
		t.Fatalf("native = %v, %v", a, err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("native SyncContext must not create files")
	}
}

func TestSyncSkillsSymlinkThenIdempotent(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}

	a, err := SyncSkills(root, sloopDir, m)
	if err != nil || a != ActionLinked {
		t.Fatalf("link = %v, %v", a, err)
	}
	fi, err := os.Lstat(filepath.Join(root, ".claude", "skills"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at .claude/skills: %v", err)
	}
	a, _ = SyncSkills(root, sloopDir, m)
	if a != ActionSkipped {
		t.Fatalf("idempotent skills = %v, want skipped", a)
	}
}

func TestSyncSkillsCopyFallback(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sloopDir, "skills", "review.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force symlink to fail.
	orig := symlinkFunc
	symlinkFunc = func(string, string) error { return os.ErrPermission }
	defer func() { symlinkFunc = orig }()

	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	a, err := SyncSkills(root, sloopDir, m)
	if err != nil || a != ActionCopied {
		t.Fatalf("copy fallback = %v, %v", a, err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "review.md"))
	if err != nil || string(b) != "hi" {
		t.Fatalf("copied skill missing/wrong: %q %v", string(b), err)
	}
}

func TestSyncSkillsNoTarget(t *testing.T) {
	a, err := SyncSkills(t.TempDir(), t.TempDir(), adapter.Manifest{})
	if err != nil || a != ActionNone {
		t.Fatalf("no target = %v, %v", a, err)
	}
}

func TestSyncSkillsRelativeLink(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	if a, err := SyncSkills(root, sloopDir, m); err != nil || a != ActionLinked {
		t.Fatalf("link = %v, %v", a, err)
	}
	dst, err := os.Readlink(filepath.Join(root, ".claude", "skills"))
	if err != nil || dst != filepath.Join("..", ".sloop", "skills") {
		t.Fatalf("want relative ../.sloop/skills, got %q (%v)", dst, err)
	}
}

func TestSyncSkillsHealsLegacyAbsoluteLink(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-hardening absolute symlink.
	if err := os.Symlink(filepath.Join(sloopDir, "skills"), link); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	if a, err := SyncSkills(root, sloopDir, m); err != nil || a != ActionRelinked {
		t.Fatalf("relink = %v, %v", a, err)
	}
	dst, _ := os.Readlink(link)
	if dst != filepath.Join("..", ".sloop", "skills") {
		t.Fatalf("want relative after heal, got %q", dst)
	}
}
