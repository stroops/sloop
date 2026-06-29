package tui

import (
	"os"
	"strings"
)

// AbbreviateHome rewrites a leading home directory as "~" for compact display,
// matching only at a path boundary so "/Users/melon" isn't shortened when home
// is "/Users/me". It returns the path unchanged when home can't be determined.
func AbbreviateHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	sep := string(os.PathSeparator)
	if rest, ok := strings.CutPrefix(path, home+sep); ok {
		return "~" + sep + rest
	}
	return path
}
