package commands

import (
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

func TestInferTool(t *testing.T) {
	keys := []string{"claude", "cursor", "agy", "codex"}
	if got := inferTool("claude-sec", keys); got != "claude" {
		t.Fatalf("claude-sec → %q", got)
	}
	if got := inferTool("agy", keys); got != "agy" {
		t.Fatalf("agy → %q", got)
	}
	if got := inferTool("randomthing", keys); got != "agent" {
		t.Fatalf("fallback → %q", got)
	}
}

func TestResolveAdopt(t *testing.T) {
	keys := []string{"claude", "agy"}
	// path basename → workspace; inferred tool.
	ws, tool := resolveAdopt("agy", "", "", keys, "/Users/me/code/myapp")
	if ws != "myapp" || tool != "agy" {
		t.Fatalf("ws=%q tool=%q", ws, tool)
	}
	// flags win.
	ws, tool = resolveAdopt("agy", "backend", "claude", keys, "/x/y")
	if ws != "backend" || tool != "claude" {
		t.Fatalf("flags: ws=%q tool=%q", ws, tool)
	}
	// no path, no flags → session name as workspace.
	ws, _ = resolveAdopt("foo", "", "", keys, "")
	if ws != "foo" {
		t.Fatalf("fallback ws=%q", ws)
	}
}

func TestExternalSessions(t *testing.T) {
	in := []tmux.Session{
		{Name: "api__claude"}, {Name: "agy"}, {Name: "web__cursor"}, {Name: "claude-sec"},
	}
	ext := externalSessions(in)
	if len(ext) != 2 || ext[0].Name != "agy" || ext[1].Name != "claude-sec" {
		t.Fatalf("external = %+v", ext)
	}
}
