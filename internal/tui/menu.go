package tui

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Clear homes the cursor and clears the screen, but only on a real terminal, so
// a control-center loop can redraw in place instead of stacking a fresh menu
// under the previous one. A no-op when piped/redirected (no escape junk).
func Clear() {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Print("\033[H\033[2J")
	}
}

// SelectMenu presents options and lets the user pick one with the arrow keys
// (or j/k). It returns the selected index, or -1 if cancelled (Esc/Ctrl-C/q) or
// run without a terminal. An option may contain "\r\n" to add indented
// continuation lines (e.g. a glance); only its first line gets the pointer.
func SelectMenu(prompt string, options []string) (int, error) {
	idx, key, err := Menu{Prompt: prompt, Options: options}.Run()
	if err != nil || key == 0 {
		return -1, err
	}
	return idx, nil
}

// SelectAction is SelectMenu plus extra single-byte action keys: pressing one
// returns immediately with the highlighted index and that key. The returned key
// is 13 (Enter), one of actionKeys, or 0 when cancelled (q/Esc/Ctrl-C) or run
// without a terminal. Callers can then act on the highlighted row in place.
func SelectAction(prompt string, options []string, actionKeys []byte) (int, byte, error) {
	return Menu{Prompt: prompt, Options: options, ActionKeys: actionKeys}.Run()
}

// cursorColor is the SGR code (bold cyan) for the pointer and the highlight of
// the row under the cursor, shared so the marker and its row always match.
const cursorColor = "1;36"

// HighlightMode selects how the row under the cursor is emphasized.
type HighlightMode int

const (
	// HighlightOff carries only the pointer, no recoloring (the default).
	HighlightOff HighlightMode = iota
	// HighlightRow lights the whole row in the pointer's cyan. Best for plain
	// menus (home, Run picker) where rows have no colors worth keeping.
	HighlightRow
	// HighlightFirstCol lights only the first column, leaving later columns
	// their own color. Best for rows that encode meaning in color (ps/ls status).
	HighlightFirstCol
)

// Menu is an arrow-key list selector shared by every interactive sloop screen
// (the home launcher, `ps`, `ls`, the Run picker) so they navigate, highlight,
// and space themselves identically. Prompt is drawn once above the list; Footer
// (optional) is drawn below it, separated by a blank line, and redrawn in place
// as the selection moves. TopPad adds one blank line above the prompt for
// breathing room. Highlight chooses how the selected row is emphasized.
type Menu struct {
	Prompt     string
	Footer     string
	Options    []string
	ActionKeys []byte
	Highlight  HighlightMode
	TopPad     bool
}

// Run draws the menu and blocks for a selection. See SelectAction for the
// return-value contract.
func (m Menu) Run() (int, byte, error) {
	if len(m.Options) == 0 {
		return -1, 0, nil
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// No tty (piped/CI): print a plain listing and return "no selection"
		// rather than erroring, so callers degrade gracefully.
		fmt.Println(m.Prompt)
		for _, opt := range m.Options {
			fmt.Println("  " + strings.ReplaceAll(opt, "\r\n", "\n  "))
		}
		if m.Footer != "" {
			fmt.Println(m.Footer)
		}
		return -1, 0, nil
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return -1, 0, err
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	// Use the terminal's alternate screen buffer so the menu has a clean
	// viewport with no scrollback. Without this, drawing more lines than the
	// terminal height causes scroll, and the subsequent cursor-up escape
	// overshoots (cursor-up clamps at line 1, but the menu's first row has
	// shifted up), so each arrow key press renders a fresh copy of the list
	// below the previous one instead of redrawing in place.
	fmt.Print("\033[?1049h")       // enter alternate screen
	defer fmt.Print("\033[?1049l") // exit alternate screen (restores main buffer)

	fmt.Print("\033[?25l")       // hide cursor
	defer fmt.Print("\033[?25h") // restore cursor

	// Disable autowrap (DECAWM) while the menu owns the screen. The in-place
	// redraw below (see draw/total) assumes exactly one terminal row per option
	// line; a line wider than the pane would wrap onto extra rows, desync the
	// cursor-up count, and smear the menu on every keypress. Clipping instead of
	// wrapping keeps the invariant for any content width (ls rows are short, so
	// it never showed there; ps glance lines can be long). Restore on exit.
	fmt.Print("\033[?7l")
	defer fmt.Print("\033[?7h")

	if m.TopPad {
		fmt.Print("\r\n")
	}
	fmt.Printf("%s\r\n\r\n", m.Prompt)

	selected := 0
	pointer := paint(cursorColor, "❯")

	// render returns the physical lines for one option. The first line of the
	// selected option gets the pointer; with Highlight set, its first column is
	// also recolored to the pointer's cyan, so the cursor reads as "arrow + the
	// name lit up" while every other column keeps its own color (e.g. ps status).
	render := func(i int) []string {
		var out []string
		for li, ln := range strings.Split(m.Options[i], "\n") {
			ln = strings.TrimRight(ln, "\r")
			gutter := "  "
			if li == 0 && i == selected {
				gutter = pointer + " "
				if ColorEnabled() {
					switch m.Highlight {
					case HighlightRow:
						ln = paint(cursorColor, StripANSI(ln))
					case HighlightFirstCol:
						ln = highlightFirstCol(ln)
					}
				}
			}
			out = append(out, gutter+ln)
		}
		return out
	}

	// Total physical lines is constant (the pointer doesn't change line count).
	total := 0
	for i := range m.Options {
		total += len(render(i))
	}
	// Footer occupies its own lines plus one blank separator above it.
	footerLines := 0
	if m.Footer != "" {
		footerLines = strings.Count(m.Footer, "\n") + 2
	}

	draw := func() {
		for i := range m.Options {
			for _, ln := range render(i) {
				fmt.Printf("\r\033[K%s\r\n", ln) // clear each line before writing
			}
		}
		if m.Footer != "" {
			fmt.Print("\r\033[K\r\n") // blank separator line
			for _, fl := range strings.Split(m.Footer, "\n") {
				fmt.Printf("\r\033[K%s\r\n", strings.TrimRight(fl, "\r"))
			}
		}
		fmt.Printf("\033[%dA", total+footerLines) // back to the top of the list
	}
	draw()

	// leave moves the cursor below the (still-drawn) menu before returning.
	leave := func() {
		fmt.Printf("\033[%dB\r\n", total+footerLines)
	}

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			// EOF or a closed stdin: cancel cleanly rather than spin. A real
			// terminal blocks here until a key is pressed, so this only fires
			// when input is exhausted (e.g. stdin redirected from a finished
			// stream). Treated as a cancel, not an error.
			leave()
			return -1, 0, nil
		}
		if n == 0 {
			continue
		}
		if n == 1 {
			switch buf[0] {
			case 3, 4, 27, 'q': // Ctrl-C, Ctrl-D, Esc, q
				leave()
				return -1, 0, nil
			case 13: // Enter
				leave()
				return selected, 13, nil
			case 'k': // vim up
				selected = (selected - 1 + len(m.Options)) % len(m.Options)
				draw()
			case 'j': // vim down
				selected = (selected + 1) % len(m.Options)
				draw()
			default:
				for _, k := range m.ActionKeys {
					if buf[0] == k {
						leave()
						return selected, k, nil
					}
				}
			}
			continue
		}
		if n == 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65: // Up
				selected = (selected - 1 + len(m.Options)) % len(m.Options)
				draw()
			case 66: // Down
				selected = (selected + 1) % len(m.Options)
				draw()
			}
		}
	}
}

// highlightFirstCol recolors the first column of a row (the leading run of
// non-space characters, after any leading spaces) in the pointer's bold cyan,
// leaving the rest — including its column padding and any later colored
// segments — untouched. It no-ops on a row that begins with an escape sequence
// (already colored) so it never double-wraps.
func highlightFirstCol(s string) string {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	if i >= len(s) || s[i] == '\x1b' {
		return s
	}
	j := i
	for j < len(s) && s[j] != ' ' {
		j++
	}
	return s[:i] + paint(cursorColor, s[i:j]) + s[j:]
}
