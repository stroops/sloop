package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/detect"
)

// TestDispatchRoutesToChild guards the fix for the menu doing nothing on
// selection: cobra's child.Execute() redirects back to the root and re-parses
// os.Args (empty for bare `sloop`), so it must instead set the root's args and
// re-run. This asserts that pattern actually reaches the chosen child's RunE.
func TestDispatchRoutesToChild(t *testing.T) {
	root := &cobra.Command{Use: "sloop", RunE: func(*cobra.Command, []string) error {
		t.Fatal("dispatch ran the root menu instead of the child")
		return nil
	}}

	var ran string
	for _, name := range []string{"ps", "doctor"} {
		n := name
		root.AddCommand(&cobra.Command{Use: n, RunE: func(*cobra.Command, []string) error {
			ran = n
			return nil
		}})
	}

	if err := dispatch(root, "doctor"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if ran != "doctor" {
		t.Fatalf("dispatch routed to %q, want doctor", ran)
	}
}

// TestRunChoicesAreLaunchable checks the Run picker only offers installed tools
// (and profiles), and that every row carries valid `run` arguments.
func TestRunChoicesAreLaunchable(t *testing.T) {
	manifests, err := adapter.Load()
	if err != nil {
		t.Skipf("adapter.Load: %v", err)
	}
	installed := map[string]bool{}
	for _, tl := range detect.Tools(manifests) {
		if tl.Installed {
			installed[tl.Key] = true
		}
	}

	choices := runChoices()
	for _, c := range choices {
		if len(c.args) != 2 || c.args[0] != "run" {
			t.Errorf("choice %q has invalid args %v, want [run <target>]", c.label, c.args)
			continue
		}
		target := c.args[1]
		if strings.HasPrefix(target, "@") {
			continue // a profile; validated by the profile/run commands
		}
		if !installed[target] {
			t.Errorf("Run picker offers %q, which is not an installed tool", target)
		}
	}

	// The workspace default tool, when installed, must be the first row.
	if def := defaultTool(); def != "" && installed[def] && len(choices) > 0 {
		if got := choices[0].args[1]; got != def {
			t.Errorf("first Run choice is %q, want default tool %q", got, def)
		}
	}
}

// TestHomeEntriesMapToRealCommands keeps the menu honest: every non-Quit entry
// must name a command that is actually registered on the root, or selecting it
// would error at runtime.
func TestHomeEntriesMapToRealCommands(t *testing.T) {
	have := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		have[c.Name()] = true
	}
	for _, e := range homeEntries {
		if e.cmd == "" || e.cmd == moreCmd {
			continue // Quit and the More-reference sentinel aren't subcommands
		}
		if !have[e.cmd] {
			t.Errorf("home menu entry %q maps to unregistered command %q", e.label, e.cmd)
		}
	}
}

// TestReferenceCommandsExcludesHidden makes sure the More screen never lists the
// hidden internal worker or cobra's help command.
func TestReferenceCommandsExcludesHidden(t *testing.T) {
	for _, c := range referenceCommands(rootCmd) {
		if c.Hidden {
			t.Errorf("More listed hidden command %q", c.Name())
		}
		if c.Name() == "help" || c.Name() == "internal-update-check" {
			t.Errorf("More should not list %q", c.Name())
		}
	}
}
