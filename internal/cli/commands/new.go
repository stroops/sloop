/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newCmd represents the new command
var newCmd = &cobra.Command{
	Use:   "new [name]",
	Short: "Create a new AI workspace",
	Long:  `Create a new workspace with optional AI model configuration.`,
	Args:  cobra.MaximumNArgs(1),
	Example: `  sloop new frontend-ai
  sloop new backend-api --model gpt-4
  sloop new --from-snapshot before-refactor`,
	RunE: runNew,
}

func init() {
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// newCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// newCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func RegisterNew(cmd *cobra.Command) {
	cmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	name := "unnamed"
	if len(args) > 0 {
		name = args[0]
	}

	fmt.Printf("⚓ Created workspace: %s \n", name)

	return nil
}
