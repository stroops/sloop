//go:build windows

package update

import "os/exec"

// detach is a no-op on Windows; the child is started without a console and we
// never Wait on it, so it runs independently.
func detach(cmd *exec.Cmd) {}
