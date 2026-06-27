package tui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// SelectMenu presents a list of options to the user and lets them select one using arrow keys.
// Returns the index of the selected option, or -1 if the user cancelled (Esc/Ctrl+C).
func SelectMenu(prompt string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, nil
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// Fallback to simple printing if not a terminal
		fmt.Println(prompt)
		for i, opt := range options {
			fmt.Printf("  %d: %s\n", i+1, opt)
		}
		return -1, fmt.Errorf("not a terminal")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return -1, err
	}
	defer term.Restore(fd, oldState)

	// Hide cursor
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	fmt.Printf("%s\r\n\r\n", prompt)

	selected := 0

	draw := func() {
		// Move cursor to start of options
		fmt.Printf("\r")
		linesPrinted := 0
		for i, opt := range options {
			if i == selected {
				// Use bold and cyan background/text inverted or just an arrow
				// ANSI reverse video is "\033[7m"
				fmt.Printf("\033[7m ❯ %s \033[0m\r\n", opt)
			} else {
				fmt.Printf("   %s \r\n", opt)
			}
			linesPrinted += 1
			for i := 0; i < len(opt); i++ {
				if opt[i] == '\n' {
					linesPrinted++
				}
			}
		}
		// Move cursor back up
		if linesPrinted > 0 {
			fmt.Printf("\033[%dA", linesPrinted)
		}
	}

	draw()

	buf := make([]byte, 3)
	for {
		n, _ := os.Stdin.Read(buf)
		if n == 0 {
			continue
		}

		if n == 1 {
			switch buf[0] {
			case 3, 4, 27: // Ctrl+C, Ctrl+D, Esc
				// Move down to the end before exiting
				fmt.Printf("\033[%dB\r\n", len(options))
				return -1, nil
			case 13: // Enter
				fmt.Printf("\033[%dB\r\n", len(options))
				return selected, nil
			}
		}

		if n == 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65: // Up
				selected--
				if selected < 0 {
					selected = len(options) - 1
				}
				draw()
			case 66: // Down
				selected++
				if selected >= len(options) {
					selected = 0
				}
				draw()
			}
		}
	}
}
