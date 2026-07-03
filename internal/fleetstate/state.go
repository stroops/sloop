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

	// RateLimitPct is 5h-rate-limit usage 0–100 (0 = unknown), from a
	// provider's statusline feed — not launch-time info, so it has no other
	// source. RateLimitReset is a short human duration ("45m", "" = unknown).
	RateLimitPct   int    `json:"rate_limit_pct,omitempty"`
	RateLimitReset string `json:"rate_limit_reset,omitempty"`
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

// Load reads a session's raw marker once, zero State when absent/corrupt.
// Hot paths (a status bar render, a ps row) call it a single time and derive
// every view from the State methods instead of re-reading the file per field.
func Load(session string) State {
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

// StatusFresh returns the status and whether it is fresh (within TTL). A stale
// or missing status yields ok=false so the caller uses its pane fallback.
func (s State) StatusFresh() (string, bool) {
	if s.UpdatedAt.IsZero() {
		return s.Status, false
	}
	return s.Status, time.Since(s.UpdatedAt) <= TTL
}

// DisplayInfo returns the model and context percentage for display. The model
// has no TTL (it doesn't go stale on its own); the context percentage is
// zeroed once the info is older than TTL, since a dead session's last reading
// would otherwise mislead.
func (s State) DisplayInfo() (model string, ctxPct int) {
	if time.Since(s.InfoAt) > TTL {
		return s.Model, 0
	}
	return s.Model, s.ContextPct
}

// DisplayRateLimit returns 5h-rate-limit usage for display, zeroed once the
// info is older than TTL (same staleness policy as DisplayInfo).
func (s State) DisplayRateLimit() (pct int, resetIn string) {
	if time.Since(s.InfoAt) > TTL {
		return 0, ""
	}
	return s.RateLimitPct, s.RateLimitReset
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

// Update applies fn to a session's marker in a single load-save round trip,
// so a caller with several fields to change (the statusline feed) doesn't
// rewrite the file once per field.
func Update(session string, fn func(*State)) error {
	s := Load(session)
	fn(&s)
	return save(session, s)
}

// Write records status for a session, stamping the current time. Enrichment
// info already in the marker is preserved.
func Write(session, status string) error {
	return Update(session, func(s *State) {
		s.Status = status
		s.UpdatedAt = time.Now()
	})
}

// WriteInfo records enrichment info for a session without touching its status.
// Empty model / non-positive ctxPct leave the existing value in place, so a
// launch-time model survives a feed that only knows the context percentage.
func WriteInfo(session, model string, ctxPct int) error {
	return Update(session, func(s *State) {
		s.SetInfo(model, ctxPct)
		s.InfoAt = time.Now()
	})
}

// SetInfo folds new model/context info into the state, keeping existing values
// when the update doesn't know them (empty model / non-positive pct).
func (s *State) SetInfo(model string, ctxPct int) {
	if model != "" {
		s.Model = model
	}
	if ctxPct > 0 {
		s.ContextPct = ctxPct
	}
}

// WriteRateLimit records 5h-rate-limit usage for a session, independent of
// status and the model/context info (its freshness reuses InfoAt, since it's
// written by the same feed call). A non-positive pct leaves the existing value
// in place.
func WriteRateLimit(session string, pct int, resetIn string) error {
	return Update(session, func(s *State) {
		s.SetRateLimit(pct, resetIn)
		s.InfoAt = time.Now()
	})
}

// SetRateLimit folds new rate-limit info into the state, keeping the existing
// values when the update doesn't know them (non-positive pct).
func (s *State) SetRateLimit(pct int, resetIn string) {
	if pct > 0 {
		s.RateLimitPct = pct
		s.RateLimitReset = resetIn
	}
}

// Read returns a session's marker and whether a *fresh* status exists (within
// TTL). A stale or missing marker yields ok=false so the caller uses its
// fallback.
func Read(session string) (State, bool) {
	s := Load(session)
	_, ok := s.StatusFresh()
	return s, ok
}

// Info and RateLimit are one-shot display reads (Load + the State accessor)
// for callers outside the render hot path.
func Info(session string) (model string, ctxPct int) {
	return Load(session).DisplayInfo()
}

func RateLimit(session string) (pct int, resetIn string) {
	return Load(session).DisplayRateLimit()
}

// Remove deletes a session's marker (best-effort): once the session is killed
// the marker is garbage, so `sloop kill` clears it right away.
func Remove(session string) {
	dir, err := Dir()
	if err != nil {
		return
	}
	_ = os.Remove(filepath.Join(dir, filename(session)))
}

// pruneAge is how long a marker whose session is gone survives before Prune
// deletes it. Generous on purpose: it only exists so a reboot (tmux sessions
// die, markers stay) followed by `sloop restore` doesn't lose the cached model
// names of sessions about to come back.
const pruneAge = 24 * time.Hour

// Prune deletes markers for sessions that are no longer running and whose file
// hasn't been touched for pruneAge, so ~/.sloop/state doesn't accumulate one
// file per session name forever. live is the current tmux session list;
// best-effort, returns how many files it removed.
func Prune(live []string) int {
	dir, err := Dir()
	if err != nil {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	keep := make(map[string]bool, len(live))
	for _, s := range live {
		keep[filename(s)] = true
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || keep[e.Name()] || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < pruneAge {
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			n++
		}
	}
	return n
}
