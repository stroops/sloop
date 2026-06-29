package commands

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

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
		return "no profiles yet; add one with `sloop profile add <name> --tool <tool> --env KEY=VAL`"
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
var profileConfigDir string

var profileCmd = &cobra.Command{
	Use:     "profile",
	Aliases: []string{"prof", "profiles"},
	Short:   "Manage reusable run profiles (e.g. a second account) in ~/.sloop/config.yaml",
	Long: `Profiles are saved launch shortcuts: a tool plus optional env vars, stored in
~/.sloop/config.yaml. Use them to switch accounts, configs, or tools without
re-typing flags every time.

Run a profile:  sloop run @<name>
List profiles:  sloop profile ls
Remove:         sloop profile rm <name>

Typical use-cases:
  • A second Claude account  →  --env CLAUDE_CONFIG_DIR=~/.claude-work
  • A Gemini key override    →  --env GEMINI_API_KEY=$GEMINI_WORK_KEY
  • A specific model alias   →  combine with sloop run @<name> -m <model>`,
}

var profileAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add or overwrite a profile: `sloop run @<name>` launches its tool with its env",
	Long: `Save a named profile to ~/.sloop/config.yaml. Once saved, launch it with
` + "`sloop run @<name>`" + `, which starts the tool in a new tmux session with the
specified env vars already set — the AI provider CLI starts automatically.

Each env value is expanded at run time: ~/... expands to the home directory and
$VAR/${VAR} references the current shell environment.

Examples:

  # A second Claude account (work config dir):
  sloop profile add work --tool claude --env CLAUDE_CONFIG_DIR=~/.claude-work
  sloop run @work                         # opens Claude with the work account

  # Multiple env vars:
  sloop profile add sec --tool gemini \
    --env GEMINI_API_KEY=$GEMINI_SEC_KEY \
    --env GEMINI_PROJECT=my-project
  sloop run @sec                          # opens Gemini with those vars

  # Override env for a one-off run without saving a profile:
  sloop run claude --env CLAUDE_CONFIG_DIR=~/.claude-work`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifests, err := adapter.Load()
		if err != nil {
			return err
		}
		g, err := config.LoadGlobal()
		if err != nil {
			return err
		}

		tool, env := profileTool, profileEnv
		var account adapter.AccountSpec
		if profileConfigDir != "" {
			key, spec, err := resolveAccountTool(profileTool, manifests)
			if err != nil {
				return err
			}
			tool, account = key, spec
			// Translate the friendly --config-dir into the tool's own env var;
			// stored raw so run-time `~`/`$VAR` expansion stays consistent.
			env = append(append([]string{}, profileEnv...), spec.ConfigDirEnv+"="+profileConfigDir)
		}

		if err := profileAdd(g, args[0], tool, env, manifests); err != nil {
			return err
		}
		if err := config.SaveGlobal(g); err != nil {
			return err
		}
		cmd.Printf("profile %q saved. Run it with `sloop run @%s`\n", args[0], args[0])

		if profileConfigDir != "" {
			ix := initInteraction(cmd)
			interactive := term.IsTerminal(int(os.Stdin.Fd())) && !ix.Auto && !ix.NoInput
			if interactive || ix.Auto {
				setupAccountDir(ix, account, profileConfigDir, interactive, bufio.NewReader(cmd.InOrStdin()), cmd.OutOrStdout())
			} else {
				cmd.Printf("(skipped config-dir setup; run interactively to create %s and share tooling)\n", profileConfigDir)
			}
		}
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
	profileAddCmd.Flags().StringVar(&profileTool, "tool", "", "the AI tool this profile launches (inferred when --config-dir names a single supporting tool)")
	profileAddCmd.Flags().StringVar(&profileConfigDir, "config-dir", "", "a second account's config dir (e.g. ~/.claude-work); sloop maps it to the tool's account env var and offers to create it + share tooling")
	profileAddCmd.Flags().StringArrayVar(&profileEnv, "env", nil, "env for the launched tool, KEY=VAL (e.g. CLAUDE_CONFIG_DIR); repeatable")
	// Either name the tool, or name a --config-dir (which infers the tool).
	profileAddCmd.MarkFlagsOneRequired("tool", "config-dir")
	_ = profileAddCmd.RegisterFlagCompletionFunc("tool", completeTools)
	profileCmd.AddCommand(profileAddCmd, profileLsCmd, profileRmCmd)
	cmd.AddCommand(profileCmd)
}
