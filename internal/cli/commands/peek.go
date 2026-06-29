package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/tmux"
)

// soleWaiting returns the session name when exactly one row is waiting, so
// `sloop peek` with no argument can jump straight to the agent that needs you.
func soleWaiting(rows []FleetRow) (string, bool) {
	name, n := "", 0
	for _, r := range rows {
		if r.Status == tmux.StatusWaiting {
			name = r.Name
			n++
		}
	}
	if n == 1 {
		return name, true
	}
	return "", false
}

// resolvePeekTarget chooses which agent to peek: an explicit arg wins; else a
// lone waiting agent is auto-selected; 0 or many fall back to the fleet picker
// (already sorted waiting-first).
func resolvePeekTarget(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	manifests, _ := adapter.Load()
	rows := enrichGlances(fleetRows(tmux.ParseSessions(tmuxList()), manifests), manifests)
	if len(rows) == 0 {
		return "", fmt.Errorf("no running AI sessions to peek (start one with `sloop run <tool>`)")
	}
	if name, ok := soleWaiting(rows); ok {
		return name, nil
	}
	return pickFleetSession("👀 Peek into which agent?")
}

// requirePeekEnv guards peek: it only makes sense as an overlay inside tmux ≥ 3.2.
func requirePeekEnv() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("`sloop peek` overlays an agent inside tmux; outside tmux use `sloop attach`")
	}
	if !tmux.PopupSupported() {
		return fmt.Errorf("`sloop peek` needs tmux ≥ 3.2 for display-popup; use `sloop attach` instead")
	}
	return nil
}

var peekInPopup bool

var peekCmd = &cobra.Command{
	Use:     "peek [agent]",
	Aliases: []string{"pk"},
	Short:   "Overlay a waiting agent, answer it, and drop back, without losing your spot",
	Long: `Float a running agent's live session in a tmux popup over your current
pane: answer the prompt it is blocked on, then close the popup and you are
exactly where you were, unlike ` + "`sloop attach`" + `, which switches your whole
screen to the agent. With no argument, peek jumps straight to the lone waiting
agent, or shows the fleet picker when several (or none) are waiting. Bind it to
a key with ` + "`sloop peek setup`" + `. Needs tmux ≥ 3.2.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requirePeekEnv(); err != nil {
			return err
		}
		target, err := resolvePeekTarget(args)
		if err != nil || target == "" {
			return err
		}
		if peekInPopup {
			// Already inside the keybind's popup: attach directly, no second popup.
			return tmux.NestedAttach(target)
		}
		return tmux.Peek(target)
	},
}

var peekKey string

var peekSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Bind <prefix> <key> to peek the waiting agent (and print the .tmux.conf line)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tmux.PopupSupported() {
			return fmt.Errorf("`sloop peek setup` needs tmux ≥ 3.2")
		}
		exe, err := os.Executable()
		if err != nil {
			exe = "sloop"
		}
		inner := exe + " peek --in-popup"
		bindLine := fmt.Sprintf(`bind-key %s display-popup -w 90%% -h 80%% -E "%s"`, peekKey, inner)
		if tmux.Available() {
			if err := tmux.BindPeek(peekKey, inner); err == nil {
				cmd.Printf("bound for this tmux server: press your prefix then %s to peek the waiting agent\n", peekKey)
			} else {
				cmd.Printf("warning: failed to bind for this tmux server: %v\n", err)
			}
		}
		cmd.Printf("add this to ~/.tmux.conf to make it permanent:\n  %s\n", bindLine)
		return nil
	},
}

func RegisterPeek(cmd *cobra.Command) {
	peekCmd.Flags().BoolVar(&peekInPopup, "in-popup", false, "internal: already inside a popup, attach directly")
	_ = peekCmd.Flags().MarkHidden("in-popup")
	peekSetupCmd.Flags().StringVar(&peekKey, "key", "p", "tmux key to bind (pressed after your prefix)")
	peekCmd.AddCommand(peekSetupCmd)
	peekCmd.ValidArgsFunction = completeSessionNames
	cmd.AddCommand(peekCmd)
}
