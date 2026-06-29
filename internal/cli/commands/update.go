package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/tui"
	"github.com/stroops/sloop/internal/update"
)

// installMethod is how this sloop binary got onto the machine, which decides how
// `sloop update` should upgrade it.
type installMethod int

const (
	methodUnknown installMethod = iota
	methodHomebrew
	methodGoInstall
)

// detectInstallMethod inspects the running executable's path. Homebrew installs
// live under the brew prefix (a "/Cellar/" path or the `brew --prefix` tree);
// `go install` builds land in GOBIN / $GOPATH/bin / ~/go/bin. Anything else
// (a hand-built `make build`, a downloaded archive) is unknown, and we fall back
// to printing instructions rather than guessing.
func detectInstallMethod(exe string) installMethod {
	exe = resolve(exe)

	if strings.Contains(exe, "/Cellar/") || strings.Contains(exe, "/homebrew/") ||
		strings.Contains(exe, "/linuxbrew/") {
		return methodHomebrew
	}
	if prefix := brewPrefix(); prefix != "" && strings.HasPrefix(exe, resolve(prefix)+string(os.PathSeparator)) {
		return methodHomebrew
	}

	exeDir := resolve(filepath.Dir(exe))
	for _, dir := range goBinDirs() {
		if dir != "" && exeDir == resolve(dir) {
			return methodGoInstall
		}
	}
	return methodUnknown
}

// resolve returns the symlink-resolved absolute path, falling back to the input
// when it can't be resolved (so detection degrades instead of erroring).
func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func brewPrefix() string {
	out, err := exec.Command("brew", "--prefix").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func goBinDirs() []string {
	var dirs []string
	if b := os.Getenv("GOBIN"); b != "" {
		dirs = append(dirs, b)
	}
	if gp := os.Getenv("GOPATH"); gp != "" {
		for _, p := range filepath.SplitList(gp) {
			dirs = append(dirs, filepath.Join(p, "bin"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "go", "bin"))
	}
	return dirs
}

// runUpdate upgrades sloop the way it was installed, delegating to the right
// package manager rather than self-replacing a managed binary. run lets tests
// stub command execution.
func runUpdate(w io.Writer, run func(name string, args ...string) error) error {
	latest, available := update.Status(version)

	cur := version
	if !update.IsRelease(cur) {
		_, _ = fmt.Fprintln(w, tui.Grey("This looks like a dev build ("+cur+"); update only applies to released versions."))
	}
	if available {
		_, _ = fmt.Fprintf(w, "Updating sloop %s → %s…\n\n", cur, latest)
	} else {
		_, _ = fmt.Fprintf(w, "Checking for a newer sloop (current: %s)…\n\n", cur)
	}

	exe, err := os.Executable()
	if err != nil {
		exe = "sloop"
	}

	switch detectInstallMethod(exe) {
	case methodHomebrew:
		_, _ = fmt.Fprintln(w, tui.Grey("$ brew upgrade sloop"))
		if err := run("brew", "upgrade", "sloop"); err != nil {
			return fmt.Errorf("brew upgrade failed: %w", err)
		}
	case methodGoInstall:
		_, _ = fmt.Fprintln(w, tui.Grey("$ go install github.com/stroops/sloop/cmd/sloop@latest"))
		if err := run("go", "install", "github.com/stroops/sloop/cmd/sloop@latest"); err != nil {
			return fmt.Errorf("go install failed: %w", err)
		}
	default:
		printManualInstructions(w, latest)
		return nil
	}

	_, _ = fmt.Fprintln(w, "\n"+tui.Green("✓ sloop updated. Run `sloop version` to confirm."))
	return nil
}

func printManualInstructions(w io.Writer, latest string) {
	_, _ = fmt.Fprintln(w, "Couldn't tell how this sloop was installed, so it can't self-update safely.")
	_, _ = fmt.Fprintln(w, "Update with whichever you used:")
	_, _ = fmt.Fprintln(w, "  Homebrew:    "+tui.Bold("brew upgrade sloop"))
	_, _ = fmt.Fprintln(w, "  Go:          "+tui.Bold("go install github.com/stroops/sloop/cmd/sloop@latest"))
	rel := "https://github.com/stroops/sloop/releases/latest"
	if latest != "" {
		rel = "https://github.com/stroops/sloop/releases/tag/v" + latest
	}
	_, _ = fmt.Fprintln(w, "  Binary:      "+tui.Grey(rel))
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update sloop to the latest release",
	Long: `Update sloop to the latest release.

sloop detects how it was installed and upgrades the same way: Homebrew installs
run "brew upgrade sloop", go-install builds re-run "go install ...@latest", and
anything else prints the right command rather than overwriting a managed binary.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpdate(cmd.OutOrStdout(), func(name string, a ...string) error {
			c := exec.Command(name, a...)
			c.Stdout, c.Stderr, c.Stdin = cmd.OutOrStdout(), cmd.ErrOrStderr(), os.Stdin
			return c.Run()
		})
	},
}

// internalUpdateCheckCmd is the detached worker spawned by the background update
// check. It performs the network call and writes the cache, then exits. Hidden
// because it's an implementation detail, not a user command.
var internalUpdateCheckCmd = &cobra.Command{
	Use:    "internal-update-check",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		update.RunCheck(version)
		return nil
	},
}

func RegisterUpdate(cmd *cobra.Command) {
	cmd.AddCommand(updateCmd)
	cmd.AddCommand(internalUpdateCheckCmd)
}
