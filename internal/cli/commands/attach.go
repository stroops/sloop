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
	Use:     "attach <session>",
	Aliases: []string{"a"},
	Short:   "Attach to a tmux session created by sloop",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunAttach(args[0])
	},
}

func RegisterAttach(cmd *cobra.Command) {
	attachCmd.ValidArgsFunction = completeSessionNames
	cmd.AddCommand(attachCmd)
}
