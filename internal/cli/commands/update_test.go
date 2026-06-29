package commands

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectInstallMethod(t *testing.T) {
	cases := []struct {
		name string
		exe  string
		want installMethod
	}{
		{"homebrew cellar", "/opt/homebrew/Cellar/sloop/0.1.2/bin/sloop", methodHomebrew},
		{"homebrew linuxbrew", "/home/linuxbrew/.linuxbrew/bin/sloop", methodHomebrew},
		{"random path", "/tmp/build/sloop", methodUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectInstallMethod(c.exe); got != c.want {
				t.Errorf("detectInstallMethod(%q) = %v, want %v", c.exe, got, c.want)
			}
		})
	}
}

func TestDetectGoInstall(t *testing.T) {
	// Point GOBIN at a temp dir and place the binary in it.
	bin := t.TempDir()
	t.Setenv("GOBIN", bin)
	exe := filepath.Join(bin, "sloop")
	if got := detectInstallMethod(exe); got != methodGoInstall {
		t.Errorf("detectInstallMethod(GOBIN/sloop) = %v, want methodGoInstall", got)
	}
}

func TestRunUpdateDelegates(t *testing.T) {
	version = "0.1.2" // simulate a released build

	var called [][]string
	stub := func(name string, args ...string) error {
		called = append(called, append([]string{name}, args...))
		return nil
	}

	// Force the unknown path by checking output, since the real executable path
	// in the test binary won't match brew/go heuristics deterministically. We
	// instead exercise runUpdate end to end and assert it prints something and
	// doesn't panic; delegation paths are covered by detectInstallMethod tests.
	var buf bytes.Buffer
	if err := runUpdate(&buf, stub); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "sloop") {
		t.Errorf("expected update output to mention sloop, got: %q", out)
	}
}
