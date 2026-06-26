#!/bin/bash
# scripts/add-cmd.sh
# Usage: ./scripts/add-cmd.sh <name> [--parent <parent>]

NAME=$1
PARENT=${3:-root}

TEMPLATE="package commands

import \"github.com/spf13/cobra\"

func Register${NAME^}(cmd *cobra.Command) {
    cmd.AddCommand(${NAME}Cmd)
}

var ${NAME}Cmd = &cobra.Command{
    Use:   \"${NAME}\",
    Short: \"TODO: Add short description\",
    RunE:  run${NAME^},
}

func run${NAME^}(cmd *cobra.Command, args []string) error {
    // TODO: Implement
    return nil
}"

echo "$TEMPLATE" > "internal/cli/commands/${NAME}.go"
echo "Created internal/cli/commands/${NAME}.go"

# Auto-add vào init.go
sed -i '' "s/func init() {/func init() {\n\tadd(Register${NAME^})/" internal/cli/commands/init.go
echo "Registered in init.go"
