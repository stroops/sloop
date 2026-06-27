package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tui"
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
	defer func() { _ = store.Close() }()

	workspaces, err := store.ListWorkspaces()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "Workspaces:")
	for _, ws := range workspaces {
		_, _ = fmt.Fprintf(w, "  %-16s %s\n", ws.Name, ws.Path)
	}

	sessions, err := store.ListSessions(10)
	if err != nil {
		return err
	}
	if len(sessions) > 0 {
		_, _ = fmt.Fprintln(w, "Recent sessions:")
		for _, s := range sessions {
			_, _ = fmt.Fprintf(w, "  %s  tool=%s  %s\n", s.StartedAt.Format("2006-01-02 15:04"), s.Tool, s.Cwd)
		}
	}
	return nil
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List sloop workspaces and recent sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := config.GlobalDBPath()
		if err != nil {
			return err
		}
		store, err := session.Open(dbPath)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()

		workspaces, err := store.ListWorkspaces()
		if err != nil {
			return err
		}
		if len(workspaces) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No workspaces found.")
			return nil
		}

		nameW := 0
		for _, ws := range workspaces {
			if len(ws.Name) > nameW {
				nameW = len(ws.Name)
			}
		}

		var options []string
		for _, ws := range workspaces {
			options = append(options, fmt.Sprintf("%-*s  %s", nameW, ws.Name, tui.Grey(ws.Path)))
		}

		prompt := fmt.Sprintf("⚓ Sloop workspaces · %d", len(workspaces)) +
			"\r\n" + tui.Grey("  ↑/↓ move · ⏎ show cd · q quit")
		selected, err := tui.SelectMenu(prompt, options)
		if err != nil {
			return err
		}
		if selected >= 0 {
			ws := workspaces[selected]
			fmt.Printf("\nTo jump to workspace, run:\n  cd %s\n", ws.Path)
		}
		return nil
	},
}

func RegisterLs(cmd *cobra.Command) { cmd.AddCommand(lsCmd) }
