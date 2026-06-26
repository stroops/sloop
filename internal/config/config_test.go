package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadProject(t *testing.T) {
	dir := t.TempDir()
	p := &Project{Tools: []string{"claude"}, DefaultTool: "claude"}
	if err := SaveProject(dir, p); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if _, err := filepath.Glob(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("glob: %v", err)
	}
	got, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if got.DefaultTool != "claude" || len(got.Tools) != 1 || got.Tools[0] != "claude" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestGlobalDBPath(t *testing.T) {
	p, err := GlobalDBPath()
	if err != nil {
		t.Fatalf("GlobalDBPath: %v", err)
	}
	if filepath.Base(p) != "sloop.db" {
		t.Fatalf("want sloop.db, got %s", p)
	}
}
