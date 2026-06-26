package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInitScaffolds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir()) // isolate the global DB

	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	for _, p := range []string{
		".sloop/config.yaml",
		".sloop/context/project.md",
		".sloop/profiles/claude.yaml",
		".sloop/.gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
	}
	for _, d := range []string{".sloop/skills", ".sloop/vault"} {
		if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
			t.Fatalf("expected dir %s: %v", d, err)
		}
	}
}
