// Package fleetstate stores per-session agent status markers written by AI tool
// hooks (e.g. Claude's Stop/Notification) and read by `sloop ps`. This makes
// status authoritative (the tool itself reports waiting/working/idle) instead
// of relying only on the pane-text heuristic. It is provider-respecting: sloop
// never intercepts the tool; the tool calls `sloop hook` through its own hook
// mechanism, and we just persist what it tells us.
package fleetstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TTL bounds how long a marker is trusted before `ps` falls back to the live
// pane heuristic, so a session that exited without firing its Stop hook can't
// stay "waiting" forever.
const TTL = 15 * time.Minute

// State is one session's last reported status.
type State struct {
	Status    string    `json:"status"` // "waiting" | "working" | "idle"
	UpdatedAt time.Time `json:"updated_at"`
}

// Dir is the global marker directory (~/.sloop/state); markers are cross-repo,
// matching the cross-repo fleet view.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sloop", "state"), nil
}

// filename maps a session name to a safe marker filename.
func filename(session string) string {
	var b strings.Builder
	for _, r := range session {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String() + ".json"
}

// Write records status for a session, stamping the current time.
func Write(session, status string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(State{Status: status, UpdatedAt: time.Now()})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filename(session)), data, 0o600)
}

// Read returns a session's marker and whether a *fresh* one exists (within TTL).
// A stale or missing marker yields ok=false so the caller uses its fallback.
func Read(session string) (State, bool) {
	dir, err := Dir()
	if err != nil {
		return State{}, false
	}
	data, err := os.ReadFile(filepath.Join(dir, filename(session)))
	if err != nil {
		return State{}, false
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, false
	}
	return s, time.Since(s.UpdatedAt) <= TTL
}
