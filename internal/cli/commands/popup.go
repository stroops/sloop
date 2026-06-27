package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/tmux"
)

// fleetPopupCmd is what the keybind runs: the cross-repo fleet in a popup.
const fleetPopupCmd = appName + " ps"

var popupCmd = &cobra.Command{
	Use:   "popup",
	Short: "Open the cross-repo fleet (`ps`) as a floating tmux popup (HUD)",
	Long: `Open ` + "`sloop ps`" + ` in a tmux popup over your current pane: glance at
every agent, answer or jump with one key, then the popup closes back to your
work. Bind it to a key with ` + "`sloop popup setup`" + `. Needs tmux ≥ 3.2.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("TMUX") == "" {
			return fmt.Errorf("`sloop popup` must be run inside tmux")
		}
		if !tmux.PopupSupported() {
			return fmt.Errorf("display-popup needs tmux ≥ 3.2 (yours: run `tmux -V`)")
		}
		exe, err := os.Executable()
		if err != nil {
			exe = "sloop"
		}
		return tmux.Popup(exe + " ps")
	},
}

var popupKey string

var popupSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Bind <prefix> <key> to open the fleet popup (and print the .tmux.conf line)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tmux.PopupSupported() {
			return fmt.Errorf("display-popup needs tmux ≥ 3.2")
		}
		bindLine := fmt.Sprintf(`bind-key %s display-popup -w 80%% -h 60%% -E "%s"`, popupKey, fleetPopupCmd)
		if tmux.Available() {
			if err := tmux.BindPopup(popupKey, fleetPopupCmd); err == nil {
				cmd.Printf("bound for this tmux server: press your prefix then %s to open the fleet popup\n", popupKey)
			} else {
				cmd.Printf("warning: failed to bind for this tmux server: %v\n", err)
			}
		}
		cmd.Printf("add this to ~/.tmux.conf to make it permanent:\n  %s\n", bindLine)
		return nil
	},
}

func RegisterPopup(cmd *cobra.Command) {
	popupSetupCmd.Flags().StringVar(&popupKey, "key", "g", "tmux key to bind (pressed after your prefix)")
	popupCmd.AddCommand(popupSetupCmd)
	cmd.AddCommand(popupCmd)
}
