package commands

import "github.com/spf13/cobra"

// Registry all commands here
var registry []func(*cobra.Command)

func Register(cmd *cobra.Command) {
	for _, fn := range registry {
		fn(cmd)
	}
}

func add(fn func(*cobra.Command)) {
	registry = append(registry, fn)
}

func init() {
	add(RegisterNew)
}
