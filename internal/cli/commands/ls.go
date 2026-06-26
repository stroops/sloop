package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/session"
)

func RunLs(w io.Writer) error {
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	workspaces, err := store.ListWorkspaces()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Workspaces:")
	for _, ws := range workspaces {
		fmt.Fprintf(w, "  %-16s %s\n", ws.Name, ws.Path)
	}

	sessions, err := store.ListSessions(10)
	if err != nil {
		return err
	}
	if len(sessions) > 0 {
		fmt.Fprintln(w, "Recent sessions:")
		for _, s := range sessions {
			fmt.Fprintf(w, "  %s  tool=%s  %s\n", s.StartedAt.Format("2006-01-02 15:04"), s.Tool, s.Cwd)
		}
	}
	return nil
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List sloop workspaces and recent sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunLs(cmd.OutOrStdout())
	},
}

func RegisterLs(cmd *cobra.Command) { cmd.AddCommand(lsCmd) }
