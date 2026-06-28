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
