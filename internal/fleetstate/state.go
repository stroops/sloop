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

// State is one session's last reported status, plus optional enrichment info
// (model, context usage) written at launch or by a provider's statusline feed.
// Status and info are updated independently: a status hook never clobbers the
// model, and a feed update never clobbers the status.
type State struct {
	Status    string    `json:"status"` // "waiting" | "working" | "idle"
	UpdatedAt time.Time `json:"updated_at"`

	// Model is the model this session runs (launch flag or the provider's own
	// display name). ContextPct is context-window usage 0–100 (0 = unknown).
	// InfoAt stamps the last info update so a stale ContextPct can be hidden.
	Model      string    `json:"model,omitempty"`
	ContextPct int       `json:"context_pct,omitempty"`
	InfoAt     time.Time `json:"info_at,omitzero"`
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

// load reads a session's raw marker, zero State when absent/corrupt.
func load(session string) State {
	dir, err := Dir()
	if err != nil {
		return State{}
	}
	data, err := os.ReadFile(filepath.Join(dir, filename(session)))
	if err != nil {
		return State{}
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}
	}
	return s
}

// save persists a session's marker, creating the directory as needed.
func save(session string, s State) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filename(session)), data, 0o600)
}

// Write records status for a session, stamping the current time. Enrichment
// info already in the marker is preserved.
func Write(session, status string) error {
	s := load(session)
	s.Status = status
	s.UpdatedAt = time.Now()
	return save(session, s)
}

// WriteInfo records enrichment info for a session without touching its status.
// Empty model / non-positive ctxPct leave the existing value in place, so a
// launch-time model survives a feed that only knows the context percentage.
func WriteInfo(session, model string, ctxPct int) error {
	s := load(session)
	if model != "" {
		s.Model = model
	}
	if ctxPct > 0 {
		s.ContextPct = ctxPct
	}
	s.InfoAt = time.Now()
	return save(session, s)
}

// Read returns a session's marker and whether a *fresh* status exists (within
// TTL). A stale or missing marker yields ok=false so the caller uses its
// fallback.
func Read(session string) (State, bool) {
	s := load(session)
	if s.UpdatedAt.IsZero() {
		return s, false
	}
	return s, time.Since(s.UpdatedAt) <= TTL
}

// Info returns a session's model and context percentage for display. The model
// has no TTL (it doesn't go stale on its own); the context percentage is
// zeroed once the info is older than TTL, since a dead session's last reading
// would otherwise mislead.
func Info(session string) (model string, ctxPct int) {
	s := load(session)
	if time.Since(s.InfoAt) > TTL {
		return s.Model, 0
	}
	return s.Model, s.ContextPct
}
