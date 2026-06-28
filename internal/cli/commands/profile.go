package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
)

// profileAdd validates the tool and stores (or overwrites) a profile in g. The
// tool is canonicalized to its manifest key so `sloop run @<name>` always lands
// in the right adapter.
func profileAdd(g *config.Global, name, tool string, envFlags []string, manifests map[string]adapter.Manifest) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	key, ok := toolKeyFor(tool, manifests)
	if !ok {
		return fmt.Errorf("unknown tool %q (no adapter)", tool)
	}
	env, err := parseEnvFlags(envFlags)
	if err != nil {
		return err
	}
	if g.Profiles == nil {
		g.Profiles = map[string]config.Profile{}
	}
	g.Profiles[name] = config.Profile{Tool: key, Env: env}
	return nil
}

// profileRemove deletes a profile, erroring when it does not exist.
func profileRemove(g *config.Global, name string) error {
	if _, ok := g.Profiles[name]; !ok {
		return fmt.Errorf("no profile %q to remove", name)
	}
	delete(g.Profiles, name)
	return nil
}

// renderProfileList formats the configured profiles (sorted), or a hint to add
// one when there are none.
func renderProfileList(profiles map[string]config.Profile) string {
	if len(profiles) == 0 {
		return "no profiles yet — add one with `sloop profile add <name> --tool <tool> --env KEY=VAL`"
	}
	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		p := profiles[n]
		keys := make([]string, 0, len(p.Env))
		for k := range p.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		line := fmt.Sprintf("%-12s %s", n, p.Tool)
		if len(keys) > 0 {
			line += "  env: " + strings.Join(keys, ", ")
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

var profileTool string
var profileEnv []string

var profileCmd = &cobra.Command{
	Use:     "profile",
	Aliases: []string{"prof", "profiles"},
	Short:   "Manage reusable run profiles (e.g. a second account) in ~/.sloop/config.yaml",
}

var profileAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add or overwrite a profile: `sloop run @<name>` launches its tool with its env",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifests, err := adapter.Load()
		if err != nil {
			return err
		}
		g, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		if err := profileAdd(g, args[0], profileTool, profileEnv, manifests); err != nil {
			return err
		}
		if err := config.SaveGlobal(g); err != nil {
			return err
		}
		cmd.Printf("profile %q saved — run it with `sloop run @%s`\n", args[0], args[0])
		return nil
	},
}

var profileLsCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List configured profiles",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		g, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		cmd.Print(renderProfileList(g.Profiles))
		return nil
	},
}

var profileRmCmd = &cobra.Command{
	Use:     "rm <name>",
	Aliases: []string{"remove"},
	Short:   "Remove a profile",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		g, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		if err := profileRemove(g, args[0]); err != nil {
			return err
		}
		if err := config.SaveGlobal(g); err != nil {
			return err
		}
		cmd.Printf("profile %q removed\n", args[0])
		return nil
	},
}

func RegisterProfile(cmd *cobra.Command) {
	profileAddCmd.Flags().StringVar(&profileTool, "tool", "", "the AI tool this profile launches (required)")
	profileAddCmd.Flags().StringArrayVar(&profileEnv, "env", nil, "env for the launched tool, KEY=VAL (e.g. CLAUDE_CONFIG_DIR); repeatable")
	_ = profileAddCmd.MarkFlagRequired("tool")
	_ = profileAddCmd.RegisterFlagCompletionFunc("tool", completeTools)
	profileCmd.AddCommand(profileAddCmd, profileLsCmd, profileRmCmd)
	cmd.AddCommand(profileCmd)
}
