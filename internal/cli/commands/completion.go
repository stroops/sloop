package commands

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
)

// Dynamic shell completion. Each function is best-effort: on any error it falls
// back to "no suggestions" (and never file completion), so an empty fleet or a
// missing DB simply yields nothing rather than a broken shell.

// completeTools suggests configured adapter (tool) names, e.g. for `run`/`sync`.
func completeTools(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	manifests, err := adapter.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(manifests))
	for name := range manifests {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeModels suggests the model aliases every adapter declares (run.models),
// e.g. for `run -m`. Aliases only — full API ids still work but aren't listed.
func completeModels(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	manifests, err := adapter.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]bool{}
	var models []string
	for _, m := range manifests {
		for _, a := range m.Run.Models {
			if !seen[a] {
				seen[a] = true
				models = append(models, a)
			}
		}
	}
	sort.Strings(models)
	return models, cobra.ShellCompDirectiveNoFileComp
}

// completeWorkspaces suggests registered workspace names (for -w/--workspace).
func completeWorkspaces(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return workspaceNames(), cobra.ShellCompDirectiveNoFileComp
}

func workspaceNames() []string {
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return nil
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	wss, err := store.ListWorkspaces()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(wss))
	for _, ws := range wss {
		names = append(names, ws.Name)
	}
	return names
}

// completeSessionNames suggests live session names (for `attach`). Only the
// first positional argument is a session.
func completeSessionNames(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	rows := fleetRows(tmux.ParseSessions(tmuxList()), nil) // only r.Name is used
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completePsIndex suggests the fleet number for `ps <#>`, annotated with the
// session it jumps to. The order matches the index `ps <#>` uses.
func completePsIndex(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	manifests, _ := adapter.Load()
	rows := fleetRows(tmux.ParseSessions(tmuxList()), manifests) // same order as `ps`
	out := make([]string, 0, len(rows))
	for i, r := range rows {
		out = append(out, fmt.Sprintf("%d\t%s", i+1, r.Name)) // value<TAB>description
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeSendTargets suggests session names and workspace names for the first
// argument of `send` (the message that follows isn't completed).
func completeSendTargets(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	rows := fleetRows(tmux.ParseSessions(tmuxList()), nil) // only r.Name is used
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	out = append(out, workspaceNames()...)
	return out, cobra.ShellCompDirectiveNoFileComp
}
