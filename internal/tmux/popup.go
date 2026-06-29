package tmux

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// BuildPopupArgs builds a `display-popup -E` invocation that runs command in a
// floating window over the current pane and closes when it exits.
func BuildPopupArgs(width, height, command string) []string {
	return []string{"display-popup", "-w", width, "-h", height, "-E", command}
}

// Popup opens command in a tmux popup (must be run inside a tmux client).
func Popup(command string) error {
	cmd := exec.Command(Bin(), BuildPopupArgs("80%", "60%", command)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// BuildBindArgs binds <prefix> <key> to open command in a popup.
func BuildBindArgs(key, command string) []string {
	return append([]string{"bind-key", key}, BuildPopupArgs("80%", "60%", command)...)
}

// BindPopup binds <prefix> <key> to open command in a popup on the live server.
func BindPopup(key, command string) error {
	return Run(BuildBindArgs(key, command)...)
}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)`)

// Version returns the multiplexer major/minor version (0,0 if unknown).
func Version() (major, minor int) {
	out, err := Output("-V")
	if err != nil {
		return 0, 0
	}
	return parseVersion(string(out))
}

// parseVersion extracts major.minor from a `tmux -V` line like "tmux 3.6b".
func parseVersion(s string) (major, minor int) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, 0
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	return major, minor
}

// PopupSupported reports whether display-popup is available (tmux ≥ 3.2).
func PopupSupported() bool {
	maj, min := Version()
	return maj > 3 || (maj == 3 && min >= 2)
}

// TitleSupported reports whether display-popup supports a -T title (tmux ≥ 3.3).
func TitleSupported() bool {
	maj, min := Version()
	return maj > 3 || (maj == 3 && min >= 3)
}
