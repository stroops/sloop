package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
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
	out, err := tmux.Output("display-message", "-p", "#S")
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

// hookRunE validates the state argument, drains any hook payload on stdin, and
// records the marker. It backs the hidden `hooks emit` callback that a provider's
// own lifecycle hooks invoke.
func hookRunE(cmd *cobra.Command, args []string) error {
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
}

// hooksEmitCmd is the internal callback an AI tool's own hooks invoke
// (`sloop hooks emit <state>`). It lives under the `hooks` namespace for
// consistency with `skills`; it is hidden because users never type it; it is
// wired into a provider's hook config by `sloop hooks install`.
var hooksEmitCmd = &cobra.Command{
	Use:    "emit <waiting|working|idle>",
	Short:  "Internal: record agent status (called by an AI tool's own hooks)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE:   hookRunE,
}

// notifyStateFor routes a provider notify payload to a sloop state by its
// `type` field — the only field sloop reads (privacy: never the messages the
// payload may carry). Unknown or malformed payloads yield "": a notify
// program must never break the host tool, so there is no error path.
func notifyStateFor(h adapter.HooksSpec, payload []byte) string {
	var p struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(payload, &p) != nil || p.Type == "" {
		return ""
	}
	switch p.Type {
	case h.Events.Working.Event:
		return "working"
	case h.Events.Waiting.Event:
		return "waiting"
	case h.Events.Idle.Event:
		return "idle"
	}
	return ""
}

// hooksNotifyCmd is the single-program variant of `hooks emit`, for tools
// (codex) that pass every event to one notify command with a JSON payload
// appended as the final argument.
var hooksNotifyCmd = &cobra.Command{
	Use:    "notify <tool> [payload-json]",
	Short:  "Internal: route a tool's notify payload to a status marker",
	Hidden: true,
	Args:   cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return nil // no payload → nothing to record, never an error
		}
		m, err := manifestForTool(args[0])
		if err != nil {
			slog.Debug("hooks notify: lookup failed", "err", err)
			return nil
		}
		state := notifyStateFor(m.Hooks, []byte(args[1]))
		if state == "" {
			slog.Debug("hooks notify: no state for payload")
			return nil
		}
		return RunHook(state)
	},
}
