package commands

import (
	"github.com/spf13/cobra"
)

var (
	newFlags  launchFlags
	newAttach bool
)

var newCmd = &cobra.Command{
	Use:   "new [tool | @profile | tool@instance] [-- <args>]",
	Short: "Create an agent session without attaching (sloop's `tmux new -d`)",
	Long: `Create an agent session and leave it running in the background.

` + "`sloop new`" + ` is ` + "`sloop run`" + ` without the attach: same context sync, same
targets and flags, but your terminal stays free — spawn a fleet from one
shell. If the session already exists it is left alone ("already running").

  sloop new claude              # spawn claude detached (or report it's running)
  sloop new claude -a           # ...and attach (same as sloop run claude)
  sloop new claude -N           # always a fresh instance (claude·2, claude·3, …)
  sloop new claude -t "fix CI"  # spawn it already working on a task
  sloop new                     # the workspace's default tool

Come back with ` + "`sloop ps`" + ` and ` + "`sloop attach`" + `. Requires tmux.`,
	Args: func(cmd *cobra.Command, args []string) error {
		return maxOneTarget(cmd, args, "")
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		positional, passthrough := splitDashArgs(cmd, args)
		return executeLaunch(cmd, positional, passthrough, launchSettings{
			launchFlags: newFlags,
			detached:    !newAttach,
		})
	},
}

func RegisterNew(cmd *cobra.Command) {
	addLaunchFlags(newCmd, &newFlags)
	newCmd.Flags().BoolVarP(&newAttach, "attach", "a", false, "attach after creating (same as `sloop run`)")
	cmd.AddCommand(newCmd)
}
