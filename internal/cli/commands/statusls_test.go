package commands

import (
	"strings"
	"testing"
)

func TestRunStatusShowsWorkspaceAndStale(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	var b strings.Builder
	if err := RunStatus(dir, &b); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, filepathBase(dir)) || !strings.Contains(out, "skills:") {
		t.Fatalf("status missing workspace/skills:\n%s", out)
	}
}

func TestRunLsListsWorkspace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir, false); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	var b strings.Builder
	if err := RunLs(&b); err != nil {
		t.Fatalf("RunLs: %v", err)
	}
	if !strings.Contains(b.String(), filepathBase(dir)) {
		t.Fatalf("ls missing workspace:\n%s", b.String())
	}
}

func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
