package commands

import "strings"

// dotSep is the plain-text field separator used throughout the fleet view
// and status bar (ps.go's tables, legends, hints). One separator, one place
// to change spacing or the glyph — before this, some call sites used " · ",
// others "  ·  " or "   ·   ", with no reason for the difference.
const dotSep = " · "

// joinWith joins non-empty items with sep, silently dropping "" entries so
// callers never end up with a stray leading/trailing/doubled separator from
// an optional field that happened to be absent. This is the one place every
// status/fleet renderer composes segments — statusline.go's tmux bar,
// statuslinefeed.go's ANSI line, and ps.go's tables all route through it (or
// joinWithDot/joinWithSpace below) instead of hardcoding a separator inline.
func joinWith(sep string, items ...string) string {
	nonEmpty := make([]string, 0, len(items))
	for _, it := range items {
		if it != "" {
			nonEmpty = append(nonEmpty, it)
		}
	}
	return strings.Join(nonEmpty, sep)
}

// joinWithDot joins with dotSep, e.g. "Opus · ctx 45% · waiting on you".
func joinWithDot(items ...string) string {
	return joinWith(dotSep, items...)
}

// joinWithSpace joins with a single space, skipping empty items.
func joinWithSpace(items ...string) string {
	return joinWith(" ", items...)
}
