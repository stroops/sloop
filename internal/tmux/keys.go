package tmux

import (
	"os"
	"strings"
)

// peekKeyCandidates / hudKeyCandidates are the prefix keys EnsureFleetKeys tries
// to bind, in priority order. The first candidate not already bound wins, so a
// user's own binding is never clobbered. `f`/`p`/`g`/`G` are tmux defaults and
// are usually skipped; `j`/`h` are free in a default tmux.
var (
	peekKeyCandidates = []string{"j", "a", "f", "p"}
	hudKeyCandidates  = []string{"h", "g", "G"}
)

// keysEnabled reports whether sloop may auto-bind the fleet keys. Power users who
// manage their own bindings opt out with SLOOP_KEYS=0 (off/false/no), mirroring
// SLOOP_STATUSLINE.
func keysEnabled() bool {
	switch strings.ToLower(os.Getenv("SLOOP_KEYS")) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

// parsePrefixKeys reads the key column out of `list-keys -T prefix` output into a
// set. tmux prints lines like `bind-key -T prefix j <command…>`; we take the
// token right after `prefix`. Lines without that shape are skipped.
func parsePrefixKeys(raw string) map[string]bool {
	keys := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(line)
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "prefix" {
				keys[f[i+1]] = true
				break
			}
		}
	}
	return keys
}

// pickFreeKey returns the first candidate not already bound, or "" if all taken.
func pickFreeKey(candidates []string, bound map[string]bool) string {
	for _, k := range candidates {
		if !bound[k] {
			return k
		}
	}
	return ""
}

// userOption reads a global tmux user option value, "" on error/unset.
func userOption(name string) string {
	out, err := Output("show-options", "-gv", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// setUserOption sets a global tmux user option (best-effort).
func setUserOption(name, val string) {
	_ = Run("set-option", "-g", name, val)
}

// PeekKey returns the prefix key sloop bound to peek (from @sloop_peek_key), or
// "" if the fleet keys aren't set up on this server. The status bar reads it to
// show "→ <prefix> <key>".
func PeekKey() string {
	return userOption("@sloop_peek_key")
}

// BoundPrefixKeys returns the set of keys already bound in the prefix table.
func BoundPrefixKeys() map[string]bool {
	out, err := Output("list-keys", "-T", "prefix")
	if err != nil {
		return map[string]bool{}
	}
	return parsePrefixKeys(string(out))
}

// EnsureFleetKeys binds the peek + hud popups to free prefix keys once per tmux
// server, recording the chosen keys in @sloop_peek_key / @sloop_hud_key so the
// status bar can show them. It is triggered at session create (next to
// SetStatusLine) but is server-global and idempotent: a no-op if the keys are
// already set up, if popups are unsupported (tmux < 3.2), or if disabled via
// SLOOP_KEYS. A key already bound by the user is skipped, never clobbered.
func EnsureFleetKeys() {
	if !keysEnabled() || !PopupSupported() {
		return
	}
	if PeekKey() != "" {
		return // already set up on this server
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "sloop"
	}
	bound := BoundPrefixKeys()
	if k := pickFreeKey(peekKeyCandidates, bound); k != "" {
		if err := BindPeek(k, exe+" peek --in-popup"); err == nil {
			setUserOption("@sloop_peek_key", k)
			bound[k] = true // don't hand the same key to hud
		}
	}
	if k := pickFreeKey(hudKeyCandidates, bound); k != "" {
		if err := BindPopup(k, exe+" ps"); err == nil {
			setUserOption("@sloop_hud_key", k)
		}
	}
}
