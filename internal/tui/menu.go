package tui

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// SelectMenu presents options and lets the user pick one with the arrow keys
// (or j/k). It returns the selected index, or -1 if cancelled (Esc/Ctrl-C/q) or
// run without a terminal. An option may contain "\r\n" to add indented
// continuation lines (e.g. a glance); only its first line gets the pointer.
func SelectMenu(prompt string, options []string) (int, error) {
	idx, key, err := SelectAction(prompt, options, nil)
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
	if len(options) == 0 {
		return -1, 0, nil
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// No tty (piped/CI): print a plain listing and return "no selection"
		// rather than erroring, so callers degrade gracefully.
		fmt.Println(prompt)
		for _, opt := range options {
			fmt.Println("  " + strings.ReplaceAll(opt, "\r\n", "\n  "))
		}
		return -1, 0, nil
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return -1, 0, err
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	fmt.Print("\033[?25l")       // hide cursor
	defer fmt.Print("\033[?25h") // restore cursor

	fmt.Printf("%s\r\n\r\n", prompt)

	selected := 0
	pointer := paint("1;36", "❯") // bold cyan

	// render returns the physical lines for one option, with the pointer on the
	// first line of the selected option and a two-space gutter otherwise.
	render := func(i int) []string {
		var out []string
		for li, ln := range strings.Split(options[i], "\n") {
			ln = strings.TrimRight(ln, "\r")
			gutter := "  "
			if li == 0 && i == selected {
				gutter = pointer + " "
			}
			out = append(out, gutter+ln)
		}
		return out
	}

	// Total physical lines is constant (the pointer doesn't change line count).
	total := 0
	for i := range options {
		total += len(render(i))
	}

	draw := func() {
		for i := range options {
			for _, ln := range render(i) {
				fmt.Printf("\r\033[K%s\r\n", ln) // clear each line before writing
			}
		}
		fmt.Printf("\033[%dA", total) // back to the top of the list
	}
	draw()

	// leave moves the cursor below the (still-drawn) menu before returning.
	leave := func() {
		fmt.Printf("\033[%dB\r\n", total)
	}

	buf := make([]byte, 3)
	for {
		n, _ := os.Stdin.Read(buf)
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
				selected = (selected - 1 + len(options)) % len(options)
				draw()
			case 'j': // vim down
				selected = (selected + 1) % len(options)
				draw()
			default:
				for _, k := range actionKeys {
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
				selected = (selected - 1 + len(options)) % len(options)
				draw()
			case 66: // Down
				selected = (selected + 1) % len(options)
				draw()
			}
		}
	}
}
