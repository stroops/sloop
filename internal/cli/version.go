package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func init() {
	// Override default --version
	rootCmd.SetVersionTemplate(`{{printf "%s\n" .Version}}`)

	// Add explicit version command
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print detailed version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("sloop version %s\n", version)
		fmt.Printf("  commit:  %s\n", commit)
		fmt.Printf("  built:   %s\n", date)
		fmt.Printf("  go:      %s\n", runtime.Version())
		fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
