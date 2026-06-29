package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/tmux"
)

// renameFunc is a seam so RunAdopt can be tested without tmux.
var renameFunc = tmux.Rename

// inferTool returns the adapter key contained in the session name, else "agent".
func inferTool(sessionName string, adapterKeys []string) string {
	low := strings.ToLower(sessionName)
	for _, k := range adapterKeys {
		if strings.Contains(low, k) {
			return k
		}
	}
	return "agent"
}

// resolveAdopt computes the workspace + tool for an adopted session (pure):
// flags win; else tool is inferred from the name and workspace from the repo dir.
func resolveAdopt(sessionName, wsFlag, toolFlag string, adapterKeys []string, path string) (ws, tool string) {
	tool = toolFlag
	if tool == "" {
		tool = inferTool(sessionName, adapterKeys)
	}
	ws = wsFlag
	switch {
	case ws != "":
	case path != "":
		ws = filepath.Base(path)
	default:
		ws = sessionName
	}
	return ws, tool
}

// RunAdopt renames a running external tmux session into the sloop
// `<workspace>__<tool>` convention and registers its workspace, bringing an
// agent you started yourself into the fleet. Returns the new session name.
func RunAdopt(sessionName, wsFlag, toolFlag string) (string, error) {
	if !tmux.Available() {
		return "", fmt.Errorf("tmux is not installed; `sloop adopt` needs tmux")
	}
	if strings.Contains(sessionName, "__") {
		return "", fmt.Errorf("%q is already a sloop session", sessionName)
	}
	live := false
	for _, s := range tmux.ParseSessions(tmuxList()) {
		if s.Name == sessionName {
			live = true
			break
		}
	}
	if !live {
		return "", fmt.Errorf("no running tmux session %q (see `sloop ps --all`)", sessionName)
	}

	var keys []string
	if m, err := adapter.Load(); err == nil {
		for k := range m {
			keys = append(keys, k)
		}
	}
	path := tmux.SessionPath(sessionName)
	ws, tool := resolveAdopt(sessionName, wsFlag, toolFlag, keys, path)
	newName := tmux.SessionName(ws, tool)
	if newName == sessionName {
		return newName, nil
	}
	if err := renameFunc(sessionName, newName); err != nil {
		return "", err
	}
	tmux.SetStatusLine(newName) // give the adopted session sloop's status bar
	tmux.EnsureFleetKeys()      // bind the fleet popup keys (once per server)
	if path != "" {
		if dbPath, err := config.GlobalDBPath(); err == nil {
			if store, err := session.Open(dbPath); err == nil {
				_, _ = store.RegisterWorkspace(ws, path)
				_ = store.Close()
			}
		}
	}
	return newName, nil
}

func completeExternalSessions(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, s := range externalSessions(tmux.ParseSessions(tmuxList())) {
		names = append(names, s.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

var (
	adoptWorkspace string
	adoptTool      string
)

var adoptCmd = &cobra.Command{
	Use:   "adopt <tmux-session>",
	Short: "Bring an external tmux session into the sloop fleet",
	Long: `Rename a running tmux session you started yourself into sloop's
<workspace>__<tool> convention, so it shows up in ` + "`sloop ps`" + ` and you can
send/answer/kill it like any sloop session.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		newName, err := RunAdopt(args[0], adoptWorkspace, adoptTool)
		if err != nil {
			return err
		}
		cmd.Printf("adopted %s → %s\n", args[0], newName)
		return nil
	},
}

func RegisterAdopt(cmd *cobra.Command) {
	adoptCmd.Flags().StringVarP(&adoptWorkspace, "workspace", "w", "", "workspace name (default: the session's repo dir)")
	adoptCmd.Flags().StringVar(&adoptTool, "as", "", "tool name (default: inferred from the session name)")
	adoptCmd.ValidArgsFunction = completeExternalSessions
	cmd.AddCommand(adoptCmd)
}
