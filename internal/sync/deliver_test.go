package sync

import (
	"os"
	"path/filepath"
	"strings"
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

func TestRepairContextBacksUpForeign(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Name: "Claude Code", Context: adapter.ContextSpec{Mode: "pointer", File: "CLAUDE.md"}}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("MINE"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := RepairContext(root, m)
	if err != nil || a != ActionRepaired {
		t.Fatalf("repair = %v, %v", a, err)
	}
	// Pointer now written.
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(string(b), "AGENTS.md") {
		t.Fatalf("pointer not written: %q", string(b))
	}
	// Original preserved under a *.sloopbak-* name.
	matches, _ := filepath.Glob(filepath.Join(root, "CLAUDE.md.sloopbak-*"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 backup, got %v", matches)
	}
	if bk, _ := os.ReadFile(matches[0]); string(bk) != "MINE" {
		t.Fatalf("backup content lost: %q", string(bk))
	}
}

func TestRepairSkillsBacksUpPresentDir(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A foreign real dir occupies the target.
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "skills", "keep.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	a, err := RepairSkills(root, sloopDir, m)
	if err != nil || a != ActionRepaired {
		t.Fatalf("repair = %v, %v", a, err)
	}
	fi, err := os.Lstat(filepath.Join(root, ".claude", "skills"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target should now be a symlink: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".claude", "skills.sloopbak-*"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 skills backup, got %v", matches)
	}
}
