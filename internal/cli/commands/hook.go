package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/fleetstate"
	"github.com/stroops/sloop/internal/tmux"
)

// currentSession resolves the sloop/tmux session the hook is firing inside.
// SLOOP_SESSION wins if set; otherwise we ask tmux for the calling pane's
// session. Outside tmux there is nothing to record.
func currentSession() string {
	if s := os.Getenv("SLOOP_SESSION"); s != "" {
		return s
	}
	if os.Getenv("TMUX") == "" {
		return ""
	}
	out, err := exec.Command(tmux.Bin(), "display-message", "-p", "#S").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RunHook records the given state for the current session. It is a no-op when
// run outside a sloop session so a stray invocation never errors.
func RunHook(state string) error {
	session := currentSession()
	if session == "" {
		return nil
	}
	return fleetstate.Write(session, state)
}

var hookCmd = &cobra.Command{
	Use:    "hook <waiting|working|idle>",
	Short:  "Internal: record agent status (called by an AI tool's own hooks)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Drain any hook payload the tool sends on stdin; we don't need it, but
		// reading avoids a broken pipe on the tool's side.
		if cmd.InOrStdin() != nil {
			_, _ = io.Copy(io.Discard, cmd.InOrStdin())
		}
		state := args[0]
		switch state {
		case "waiting", "working", "idle":
		default:
			return fmt.Errorf("unknown state %q (want waiting|working|idle)", state)
		}
		return RunHook(state)
	},
}

func RegisterHook(cmd *cobra.Command) { cmd.AddCommand(hookCmd) }
