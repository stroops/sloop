package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stroops/sloop/internal/adapter"
)

// credentialsFile is never shared between accounts, whatever a manifest lists;
// it is the login itself, so sharing it would defeat having a second account.
const credentialsFile = ".credentials.json"

// resolveAccountTool picks the tool a `--config-dir` profile targets. An explicit
// tool must declare account.config_dir_env; with no tool given, the sole tool
// that declares it is used. Errors when none qualify, or when several do without
// an explicit choice.
func resolveAccountTool(tool string, manifests map[string]adapter.Manifest) (string, adapter.AccountSpec, error) {
	if tool != "" {
		key, ok := toolKeyFor(tool, manifests)
		if !ok {
			return "", adapter.AccountSpec{}, fmt.Errorf("unknown tool %q (no adapter)", tool)
		}
		spec := manifests[key].Account
		if spec.ConfigDirEnv == "" {
			return "", adapter.AccountSpec{}, fmt.Errorf("--config-dir is not supported for %q (it has no account config dir); use --env instead", tool)
		}
		return key, spec, nil
	}
	var keys []string
	for key, m := range manifests {
		if m.Account.ConfigDirEnv != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	switch len(keys) {
	case 0:
		return "", adapter.AccountSpec{}, fmt.Errorf("no tool supports --config-dir")
	case 1:
		return keys[0], manifests[keys[0]].Account, nil
	default:
		return "", adapter.AccountSpec{}, fmt.Errorf("several tools support --config-dir (%s); pass --tool to pick one", strings.Join(keys, ", "))
	}
}

// setupAccountDir does the filesystem side of `profile add --config-dir`: create
// the dir if missing, then optionally symlink shareable subpaths from the tool's
// default config dir: tooling (default yes) and, opt-in, conversation history
// for cross-account resume (default no). Best-effort and never touches
// credentials. `interactive` gates the prompts that have no safe automatic
// answer (history sharing); tooling honours --auto.
func setupAccountDir(ix Interaction, spec adapter.AccountSpec, configDir string, interactive bool, in *bufio.Reader, out io.Writer) {
	dir := expandEnvValue(configDir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if !ix.Ask(fmt.Sprintf("Create config dir %s?", dir), true, in, out) {
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_, _ = fmt.Fprintf(out, "  could not create %s: %v\n", dir, err)
			return
		}
		_, _ = fmt.Fprintf(out, "  created %s\n", dir)
	}

	src := expandEnvValue(spec.DefaultDir)
	if src == "" || src == dir {
		return
	}
	if _, err := os.Stat(src); err != nil {
		return // no default dir to share from
	}

	if len(spec.Share) > 0 &&
		ix.Ask(fmt.Sprintf("Share tooling (%s) from %s?", strings.Join(spec.Share, ", "), src), true, in, out) {
		linkShared(spec.Share, src, dir, out)
	}

	// History sharing has no safe default-yes: only offer it as a real prompt.
	if interactive && len(spec.ShareState) > 0 &&
		ix.Ask(fmt.Sprintf("Share conversation history (%s) so this account can resume the other's sessions?", strings.Join(spec.ShareState, ", ")), false, in, out) {
		linkShared(spec.ShareState, src, dir, out)
	}
}

// linkShared symlinks each item from src into dst, create-if-missing: it never
// overwrites a path already in dst, skips a missing source, and refuses
// credentials outright. It logs one line per item it touches.
func linkShared(items []string, src, dst string, out io.Writer) {
	for _, item := range items {
		if filepath.Base(item) == credentialsFile {
			continue // never share the login
		}
		target := filepath.Join(dst, item)
		if _, err := os.Lstat(target); err == nil {
			_, _ = fmt.Fprintf(out, "  skip %s (already present)\n", item)
			continue
		}
		source := filepath.Join(src, item)
		if _, err := os.Stat(source); err != nil {
			continue // nothing to share
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_, _ = fmt.Fprintf(out, "  could not link %s: %v\n", item, err)
			continue
		}
		if err := os.Symlink(source, target); err != nil {
			_, _ = fmt.Fprintf(out, "  could not link %s: %v\n", item, err)
			continue
		}
		_, _ = fmt.Fprintf(out, "  linked %s → %s\n", item, source)
	}
}
