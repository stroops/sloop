package commands

import (
	"testing"

	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
)

func TestRestoreCandidates(t *testing.T) {
	wsName := map[int64]string{1: "api", 2: "web"}
	wsPath := map[int64]string{1: "/repo/api", 2: "/repo/web"}

	// History, most-recent first: api/claude (live), api/cursor, web/claude,
	// an older duplicate api/cursor, and a session whose workspace is gone.
	sessions := []session.Session{
		{WorkspaceID: 1, Tool: "claude"},
		{WorkspaceID: 1, Tool: "cursor"},
		{WorkspaceID: 2, Tool: "claude"},
		{WorkspaceID: 1, Tool: "cursor"}, // older dup → skipped
		{WorkspaceID: 9, Tool: "claude"}, // unknown workspace → skipped
	}
	live := map[string]bool{tmux.SessionName("api", "claude"): true} // api/claude already up

	got := restoreCandidates(sessions, wsName, wsPath, live)

	// Expect api/cursor and web/claude (api/claude excluded as live, dup + unknown skipped).
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d: %+v", len(got), got)
	}
	if got[0].Tool != "cursor" || got[0].WSName != "api" || got[0].Path != "/repo/api" {
		t.Fatalf("candidate 0 = %+v", got[0])
	}
	if got[1].Tool != "claude" || got[1].WSName != "web" {
		t.Fatalf("candidate 1 = %+v", got[1])
	}
	if got[1].Session != tmux.SessionName("web", "claude") {
		t.Fatalf("session name = %q", got[1].Session)
	}
}

func TestRestoreCandidatesEmpty(t *testing.T) {
	// Every recent session is already live → nothing to restore.
	wsName := map[int64]string{1: "api"}
	wsPath := map[int64]string{1: "/repo/api"}
	sessions := []session.Session{{WorkspaceID: 1, Tool: "claude"}}
	live := map[string]bool{tmux.SessionName("api", "claude"): true}
	if got := restoreCandidates(sessions, wsName, wsPath, live); len(got) != 0 {
		t.Fatalf("want 0, got %+v", got)
	}
}
