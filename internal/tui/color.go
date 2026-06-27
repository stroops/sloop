package tui

import (
	"os"

	"golang.org/x/term"
)

// ColorEnabled reports whether ANSI color should be emitted. Color is off when
// NO_COLOR is set (https://no-color.org) or stdout is not a terminal (piped or
// redirected), and on otherwise.
func ColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func paint(code, s string) string {
	if !ColorEnabled() {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

// Color helpers. Grey uses a 256-color mid-grey (245) that stays legible on both
// light and dark themes, unlike the classic "bright black" (90) which often
// disappears on dark backgrounds.
func Bold(s string) string   { return paint("1", s) }
func Grey(s string) string   { return paint("38;5;245", s) }
func Yellow(s string) string { return paint("33", s) }
func Cyan(s string) string   { return paint("36", s) }
func Green(s string) string  { return paint("32", s) }
func Blue(s string) string   { return paint("34", s) }
