package profile

import (
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	if Default("claude").Tool != "claude" {
		t.Fatal("default tool wrong")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.yaml")
	if err := Save(path, Profile{Tool: "claude"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Tool != "claude" {
		t.Fatalf("roundtrip: %+v", got)
	}
}
