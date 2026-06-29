package tmux

import (
	"strings"
	"testing"
)

func TestShellSingleQuote(t *testing.T) {
	if got := shellSingleQuote("ws__claude"); got != "'ws__claude'" {
		t.Fatalf("plain: got %q", got)
	}
	if got := shellSingleQuote("a'b"); got != `'a'\''b'` {
		t.Fatalf("embedded quote: got %q", got)
	}
}

func TestBuildNestedAttachCmd(t *testing.T) {
	got := BuildNestedAttachCmd("ws__claude")
	want := "env -u TMUX -u TMUX_PANE " + Bin() + " attach -t 'ws__claude'"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildPeekBindArgs(t *testing.T) {
	got := BuildPeekBindArgs("p", "CMD")
	want := []string{"bind-key", "p", "display-popup", "-w", "90%", "-h", "80%", "-E", "CMD"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPeekTitle(t *testing.T) {
	if got := peekTitle("api__claude", "Ctrl+b"); got != " 👀 peek · api__claude — Ctrl+b d to close " {
		t.Fatalf("named: got %q", got)
	}
	if got := peekTitle("", "Ctrl+b"); got != " 👀 peek — Ctrl+b d to close " {
		t.Fatalf("generic: got %q", got)
	}
}

func TestWithTitle(t *testing.T) {
	base := []string{"display-popup", "-w", "90%", "-h", "80%", "-E", "CMD"}
	want := []string{"display-popup", "-w", "90%", "-h", "80%", "-T", "T", "-E", "CMD"}
	if got := withTitle(base, "T"); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := withTitle(base, ""); strings.Join(got, " ") != strings.Join(base, " ") {
		t.Fatalf("empty title should be a no-op, got %v", got)
	}
}
