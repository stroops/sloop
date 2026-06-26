package profile

import (
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	p := Default("claude")
	if p.Tool != "claude" || p.Context != "all" {
		t.Fatalf("unexpected default: %+v", p)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.yaml")
	want := Profile{Tool: "claude", Context: "all", Skills: []string{"review.md"}}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Tool != "claude" || len(got.Skills) != 1 || got.Skills[0] != "review.md" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}
