package tui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// ReadLine reads a single line in raw mode so the caller's control loop keeps
// ownership of the terminal: Enter submits (ok=true), while Esc, Ctrl-C and
// Ctrl-D cancel (ok=false) instead of killing the process. This is what lets the
// `ps` control center treat a cancelled send as "back to the fleet" rather than
// dropping to the shell. Backspace deletes a whole rune, so multibyte input
// (e.g. Vietnamese) edits cleanly. Without a tty it returns ("", false).
func ReadLine(prompt string) (string, bool) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", false
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", false
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	fmt.Print(prompt)
	var buf []byte
	in := make([]byte, 16)
	for {
		n, err := os.Stdin.Read(in)
		if err != nil {
			fmt.Print("\r\n")
			return "", false // stdin closed / hard error → cancel, like Esc (never spin)
		}
		if n == 0 {
			continue
		}
		b := in[:n]
		if n == 1 {
			switch b[0] {
			case 3, 4, 27: // Ctrl-C, Ctrl-D, Esc → cancel
				fmt.Print("\r\n")
				return "", false
			case 13, 10: // Enter → submit
				fmt.Print("\r\n")
				return string(buf), true
			case 127, 8: // Backspace → drop the last rune and redraw
				buf = trimLastRune(buf)
				fmt.Print("\r\033[K" + prompt + string(buf))
				continue
			}
			if b[0] < 32 { // other control byte (Tab, etc.) → ignore
				continue
			}
		} else if b[0] == 27 {
			continue // multi-byte escape sequence (arrow keys, …) → ignore
		}
		buf = append(buf, b...) // printable ASCII or UTF-8 multibyte
		fmt.Print(string(b))
	}
}

// Confirm asks a y/N question in raw mode and returns true only on y/Y. Any
// other key (including Esc and Ctrl-C) answers no without killing the process,
// so a declined confirm returns to the caller's control loop.
func Confirm(prompt string) bool { return ConfirmDefault(prompt, false) }

// ConfirmDefault is Confirm with a configurable default: y/Y is yes, n/N is no,
// and Enter or any other key takes def. This lets a "[Y/n]" prompt treat a bare
// Enter as yes while still never killing the process on Esc/Ctrl-C.
func ConfirmDefault(prompt string, def bool) bool {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return def
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return def
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	fmt.Print(prompt)
	in := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(in)
		if err != nil {
			return def // stdin closed / hard error → take the default (never spin)
		}
		if n == 0 {
			continue
		}
		fmt.Print("\r\n")
		switch in[0] {
		case 'y', 'Y':
			return true
		case 'n', 'N':
			return false
		default: // Enter, Esc, Ctrl-C, anything else
			return def
		}
	}
}

// trimLastRune drops the final UTF-8 rune from b (backing over continuation
// bytes), so Backspace deletes a whole character, not a stray byte.
func trimLastRune(b []byte) []byte {
	i := len(b) - 1
	for i > 0 && b[i]&0xC0 == 0x80 {
		i--
	}
	if i < 0 {
		return b[:0]
	}
	return b[:i]
}
