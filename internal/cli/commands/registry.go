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
	add(RegisterInit)
	add(RegisterSync)
	add(RegisterRun)
	add(RegisterAttach)
	add(RegisterTools)
	add(RegisterDoctor)
	add(RegisterStatus)
	add(RegisterLs)
	add(RegisterPs)
	add(RegisterSend)
	add(RegisterSkill)
}
