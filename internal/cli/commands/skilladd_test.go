package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRawURL(t *testing.T) {
	got := rawURL("https://github.com/acme/skills/blob/main/review.md")
	want := "https://raw.githubusercontent.com/acme/skills/main/review.md"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if u := rawURL("https://example.com/x.md"); u != "https://example.com/x.md" {
		t.Fatalf("plain URL changed: %q", u)
	}
}

func TestSkillNameFromURL(t *testing.T) {
	if n := skillNameFromURL("https://example.com/path/code-review.md?ref=1"); n != "code-review" {
		t.Fatalf("got %q", n)
	}
}

func TestRunSkillAddFetchesAndLinks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // claude fallback (has .claude/skills target)
	if err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	orig := fetchURL
	fetchURL = func(string) ([]byte, error) { return []byte("# Code Review\n\nDo a careful review.\n"), nil }
	defer func() { fetchURL = orig }()

	dst, linked, err := RunSkillAdd(dir, "https://example.com/code-review.md", "")
	if err != nil {
		t.Fatalf("RunSkillAdd: %v", err)
	}
	if filepath.Base(dst) != "code-review.md" {
		t.Fatalf("want code-review.md, got %s", dst)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "# Code Review\n\nDo a careful review.\n" {
		t.Fatalf("content = %q", string(b))
	}
	if len(linked) == 0 {
		t.Fatalf("expected linked into a tool")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "code-review.md")); err != nil {
		t.Fatalf("imported skill not visible via symlink: %v", err)
	}
	// Duplicate import errors.
	if _, _, err := RunSkillAdd(dir, "https://example.com/code-review.md", ""); err == nil {
		t.Fatal("expected duplicate error")
	}
}
