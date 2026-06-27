package skills

import (
	"os"
	"testing"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	l, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if l.Version != LockVersion || len(l.Skills) != 0 {
		t.Fatalf("want empty v%d lock, got %+v", LockVersion, l)
	}
}

func TestUpsertSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l, _ := Load(dir)
	l.Upsert(Entry{Name: "review", Source: "https://x/review.md", SHA256: Hash([]byte("a"))})
	l.Upsert(Entry{Name: "apply", Source: "https://x/apply.md"})
	// Replace existing by name.
	l.Upsert(Entry{Name: "review", Source: "https://x/review.md", SHA256: Hash([]byte("b"))})
	if err := l.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Skills) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got.Skills))
	}
	// Sorted by name: apply before review.
	if got.Skills[0].Name != "apply" || got.Skills[1].Name != "review" {
		t.Fatalf("not sorted: %+v", got.Skills)
	}
	if e, ok := got.Get("review"); !ok || e.SHA256 != Hash([]byte("b")) {
		t.Fatalf("upsert did not replace: %+v", e)
	}
}

func TestHashStable(t *testing.T) {
	x1, x2, y := Hash([]byte("x")), Hash([]byte("x")), Hash([]byte("y"))
	if x1 != x2 {
		t.Fatal("Hash not deterministic for identical content")
	}
	if x1 == y {
		t.Fatal("Hash collides on different content")
	}
}

func TestSavePermissions(t *testing.T) {
	dir := t.TempDir()
	l, _ := Load(dir)
	l.Upsert(Entry{Name: "a", Source: "s"})
	if err := l.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(LockPath(dir))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}
}
