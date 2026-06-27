package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".sloop"), 0o700); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	w, err := Resolve(nested)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	gotRoot, _ := filepath.EvalSymlinks(w.Root)
	wantRoot, _ := filepath.EvalSymlinks(root)
	if gotRoot != wantRoot {
		t.Fatalf("want root %s, got %s", wantRoot, gotRoot)
	}
	if w.Name != filepath.Base(wantRoot) {
		t.Fatalf("want name %s, got %s", filepath.Base(wantRoot), w.Name)
	}
}

func TestResolveNotFound(t *testing.T) {
	_, err := Resolve(t.TempDir())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
