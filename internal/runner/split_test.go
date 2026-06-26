package runner

import (
	"strings"
	"testing"
)

func TestBuildTmuxSplitNew(t *testing.T) {
	got := strings.Join(BuildTmuxSplitNew("ws__claude_cursor", "/repo", "claude"), " ")
	want := "new-session -d -s ws__claude_cursor -c /repo claude"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildTmuxSplitAdd(t *testing.T) {
	got := strings.Join(BuildTmuxSplitAdd("ws__claude_cursor", "/repo", "cursor"), " ")
	want := "split-window -t ws__claude_cursor -c /repo cursor"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildTmuxTiledLayout(t *testing.T) {
	got := strings.Join(BuildTmuxTiledLayout("ws__x"), " ")
	if got != "select-layout -t ws__x tiled" {
		t.Fatalf("got %q", got)
	}
}

func TestLaunchSplitNoCmds(t *testing.T) {
	if err := LaunchSplit("s", "/tmp", nil); err != nil {
		t.Fatalf("empty cmds should be a no-op, got %v", err)
	}
}
