//go:build !windows

package update

import (
	"os/exec"
	"syscall"
)

// detach puts the background check in its own session so it survives the parent
// command exiting (and isn't reached by a Ctrl-C aimed at the foreground).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
