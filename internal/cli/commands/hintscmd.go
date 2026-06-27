package commands

import (
	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/hints"
)

var hintsCmd = &cobra.Command{
	Use:   "hints",
	Short: "Show or toggle the contextual education tips",
	RunE:  func(cmd *cobra.Command, args []string) error { return runHintsList(cmd) },
}

var hintsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every hint in your language",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return runHintsList(cmd) },
}

func runHintsList(cmd *cobra.Command) error {
	lang := hints.Lang()
	for _, h := range hints.Load() {
		cmd.Printf("• [%s] %s\n", h.Context, h.Localized(lang))
	}
	return nil
}

func setHints(cmd *cobra.Command, on bool) error {
	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	g.Hints = &on
	if err := config.SaveGlobal(g); err != nil {
		return err
	}
	if on {
		cmd.Println("hints on")
	} else {
		cmd.Println("hints off")
	}
	return nil
}

var hintsOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Enable contextual hints",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return setHints(cmd, true) },
}

var hintsOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Disable contextual hints",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return setHints(cmd, false) },
}

func RegisterHints(cmd *cobra.Command) {
	hintsCmd.AddCommand(hintsListCmd, hintsOnCmd, hintsOffCmd)
	cmd.AddCommand(hintsCmd)
}
