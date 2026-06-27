package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/runner"
)

// attachArgs is a tiny testable seam returning the tmux args as a string.
func attachArgs(session string) string {
	return strings.Join(runner.BuildTmuxAttachArgs(session), " ")
}

func RunAttach(session string) error {
	if !runner.TmuxAvailable() {
		return fmt.Errorf("tmux is not installed; attach requires tmux")
	}
	
	fmt.Printf("\n\033[36m💡 SLOOP HINT: To safely hide this agent and return to the terminal,\033[0m\n")
	fmt.Printf("\033[36m   press \033[1mCtrl+b\033[0m\033[36m then press \033[1md\033[0m\n\n")

	cmd := exec.Command("tmux", runner.BuildTmuxAttachArgs(session)...)
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

func RegisterAttach(cmd *cobra.Command) { cmd.AddCommand(attachCmd) }
