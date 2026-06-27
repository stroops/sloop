package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/skills"
)

// addLockedSkill imports a skill via the fetch seam and returns the workspace dir.
func addLockedSkill(t *testing.T, dir, body string) {
	t.Helper()
	orig := fetchURL
	fetchURL = func(string) ([]byte, error) { return []byte(body), nil }
	defer func() { fetchURL = orig }()
	if _, _, err := RunSkillAdd(dir, "https://example.com/review.md", ""); err != nil {
		t.Fatalf("RunSkillAdd: %v", err)
	}
}

func TestSkillAddRecordsLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if _, err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	addLockedSkill(t, dir, "# Review\n\nv1\n")

	lock, err := skills.Load(filepath.Join(dir, ".sloop"))
	if err != nil {
		t.Fatalf("Load lock: %v", err)
	}
	e, ok := lock.Get("review")
	if !ok {
		t.Fatal("review not recorded in lock")
	}
	if e.Source != "https://example.com/review.md" || e.SHA256 != skills.Hash([]byte("# Review\n\nv1\n")) {
		t.Fatalf("lock entry wrong: %+v", e)
	}
}

func TestRunSkillUpdateRefetches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if _, err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	addLockedSkill(t, dir, "# Review\n\nv1\n")

	// Re-fetch with changed content → updated; file + lock hash refreshed.
	orig := fetchURL
	fetchURL = func(string) ([]byte, error) { return []byte("# Review\n\nv2\n"), nil }
	defer func() { fetchURL = orig }()

	res, err := RunSkillUpdate(dir, nil)
	if err != nil {
		t.Fatalf("RunSkillUpdate: %v", err)
	}
	if len(res.Outcomes) != 1 || !res.Outcomes[0].Updated {
		t.Fatalf("expected 1 updated outcome, got %+v", res.Outcomes)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".sloop", "skills", "review.md"))
	if string(b) != "# Review\n\nv2\n" {
		t.Fatalf("file not rewritten: %q", string(b))
	}

	// Same content again → unchanged, no rewrite.
	res, err = RunSkillUpdate(dir, nil)
	if err != nil {
		t.Fatalf("RunSkillUpdate (2): %v", err)
	}
	if res.Outcomes[0].Updated {
		t.Fatal("expected unchanged on identical content")
	}
}

func TestRunSkillUpdateUnknownName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if _, err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	if _, err := RunSkillUpdate(dir, []string{"nope"}); err == nil {
		t.Fatal("expected error for unknown skill name")
	}
}

func TestRunSkillUpdateEmptyLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if _, err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	res, err := RunSkillUpdate(dir, nil)
	if err != nil {
		t.Fatalf("RunSkillUpdate: %v", err)
	}
	if len(res.Outcomes) != 0 {
		t.Fatalf("expected no outcomes, got %+v", res.Outcomes)
	}
}
