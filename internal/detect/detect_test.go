package detect

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

func TestToolsDetectsBinaryOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary PATH trick is unix-only")
	}
	dir := t.TempDir()
	// Create a fake executable named "faketool".
	bin := filepath.Join(dir, "faketool")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho v1.2.3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	manifests := map[string]adapter.Manifest{
		"faketool": {Name: "Fake Tool", Detect: "faketool", Launch: "faketool"},
		"missing":  {Name: "Missing", Detect: "definitely-not-installed-xyz", Launch: "x"},
	}
	got := Tools(manifests)
	byKey := map[string]ToolStatus{}
	for _, s := range got {
		byKey[s.Key] = s
	}
	if !byKey["faketool"].Installed {
		t.Fatalf("faketool should be installed: %+v", byKey["faketool"])
	}
	if byKey["missing"].Installed {
		t.Fatalf("missing should not be installed: %+v", byKey["missing"])
	}
}

func TestInstalledKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "faketool"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	keys := InstalledKeys(map[string]adapter.Manifest{
		"faketool": {Detect: "faketool"},
		"missing":  {Detect: "nope-xyz"},
	})
	if len(keys) != 1 || keys[0] != "faketool" {
		t.Fatalf("want [faketool], got %v", keys)
	}
}
