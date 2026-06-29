package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/tmux"
)

// sendFunc is a seam so RunApprove can be tested without tmux.
var sendFunc = tmux.LaunchSend

// RunApprove sends each targeted waiting agent its affirmative answer (the
// parsed Yes/Approve/Continue choice). Reads the agents' own prompts, no LLM.
func RunApprove(w io.Writer, in io.Reader, targets []string, all, waiting, yes bool) ([]string, error) {
	if !tmux.Available() {
		return nil, fmt.Errorf("tmux is not installed; `sloop approve` needs tmux")
	}
	manifests, _ := adapter.Load()
	// Enrich first so each row has a Status; selectSessions does its own
	// filterWaiting for the --waiting case (mirrors kill). Pre-filtering here
	// would run before Status is set and drop every row.
	rows := enrichGlances(fleetRows(tmux.ParseSessions(tmuxList()), manifests), manifests)
	sel, err := selectSessions(rows, targets, all, waiting)
	if err != nil {
		return nil, err
	}
	if all && !yes {
		if !confirm(w, in, fmt.Sprintf("approve %d session(s)? [y/N] ", len(sel))) {
			return nil, nil
		}
	}
	var done []string
	for _, r := range sel {
		a, ok := tmux.AffirmativeAnswer(r.Answers)
		if !ok {
			continue // not a recognizable approval prompt; skip
		}
		if err := sendFunc(r.Name, a.Key); err != nil {
			return done, err
		}
		done = append(done, fmt.Sprintf("%s: %s", r.Name, a.Label))
	}
	return done, nil
}

var (
	approveAll     bool
	approveWaiting bool
	approveYes     bool
)

var approveCmd = &cobra.Command{
	Use:   "approve [<#|session|workspace>...]",
	Short: "Send the affirmative answer to waiting agent(s); one-key approve",
	Long: `Read what each waiting agent is asking and send its Yes/Approve/Continue
answer. ` + "`approve --waiting`" + ` approves every agent waiting on you at once.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		done, err := RunApprove(cmd.OutOrStdout(), cmd.InOrStdin(), args,
			approveAll, approveWaiting, assumeYes(cmd, approveYes))
		if err != nil {
			return err
		}
		if len(done) == 0 {
			cmd.Println("nothing to approve (no recognizable prompt)")
			return nil
		}
		for _, d := range done {
			cmd.Printf("approved %s\n", d)
		}
		return nil
	},
}

func RegisterApprove(cmd *cobra.Command) {
	approveCmd.Flags().BoolVar(&approveAll, "all", false, "approve every running session")
	approveCmd.Flags().BoolVar(&approveWaiting, "waiting", false, "approve every session waiting on you")
	approveCmd.Flags().BoolVar(&approveYes, "yes", false, "skip the --all confirmation (or use global -y)")
	approveCmd.ValidArgsFunction = completeSessionNames
	cmd.AddCommand(approveCmd)
}
