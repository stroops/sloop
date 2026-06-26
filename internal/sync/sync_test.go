package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/profile"
)

func TestAssembleOrdersContextThenSkills(t *testing.T) {
	sloopDir := t.TempDir()
	mustWrite(t, filepath.Join(sloopDir, "context", "a.md"), "alpha")
	mustWrite(t, filepath.Join(sloopDir, "context", "b.md"), "bravo")
	mustWrite(t, filepath.Join(sloopDir, "skills", "review.md"), "do a review")

	out, err := Assemble(sloopDir, profile.Profile{Context: "all", Skills: []string{"review.md"}})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	ia := strings.Index(out, "alpha")
	ib := strings.Index(out, "bravo")
	ir := strings.Index(out, "do a review")
	if !(ia < ib && ib < ir) {
		t.Fatalf("wrong order: a=%d b=%d review=%d\n%s", ia, ib, ir, out)
	}
	if !strings.Contains(out, "## a.md") || !strings.Contains(out, "## review.md") {
		t.Fatalf("missing source headings:\n%s", out)
	}
}

func TestWriteNativeFiles(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Outputs: []adapter.Output{{Path: "CLAUDE.md", Template: "default"}}}
	written, err := WriteNativeFiles(root, m, "hello")
	if err != nil {
		t.Fatalf("WriteNativeFiles: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 file, got %v", written)
	}
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if string(b) != "hello" {
		t.Fatalf("want hello, got %q", string(b))
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
