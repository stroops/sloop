package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stroops/sloop/internal/tmux"
)

// attachArgs is a tiny testable seam returning the tmux args as a string.
func attachArgs(session string) string {
	return strings.Join(tmux.BuildAttachArgs(session), " ")
}

func RunAttach(session string) error {
	if !tmux.Available() {
		return fmt.Errorf("tmux is not installed; attach requires tmux")
	}

	fmt.Printf("\n%s\n\n", tmux.DetachHint())

	cmd := exec.Command(tmux.Bin(), tmux.BuildAttachArgs(session)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var attachCmd = &cobra.Command{
	Use:     "attach [session]",
	Aliases: []string{"a"},
	Short:   "Attach to a sloop session (no name = pick from the fleet)",
	Long: `Attach to a tmux session sloop created. Give a session name to attach
directly, or run it with no argument (` + "`sloop a`" + `) to pick one from the
running fleet.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return RunAttach(args[0])
		}
		name, err := pickFleetSession("⚓ Attach to which agent?")
		if err != nil || name == "" {
			return err
		}
		return RunAttach(name)
	},
}

func RegisterAttach(cmd *cobra.Command) {
	attachCmd.ValidArgsFunction = completeSessionNames
	cmd.AddCommand(attachCmd)
}
