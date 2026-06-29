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
