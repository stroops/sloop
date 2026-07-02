package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/stroops/sloop/internal/adapter"
)

// Codex has exactly one notify slot (config.toml `notify = [cmd, args…]`,
// invoked with a JSON payload appended; see
// https://developers.openai.com/codex/config-advanced). Sloop only claims the
// slot when it's free: an occupied slot is the user's, and install reports
// errNotifyOccupied so the command can print chaining guidance instead.
//
// The write is a textual prepend, not a TOML re-marshal: go-toml/v2 drops
// comments/formatting on round-trip, and a top-level key appended at the end
// of the file would land inside the last [table]. Prepending the one line
// keeps the rest of the user's config byte-identical.

// errNotifyOccupied reports that codex's notify slot already runs something
// that isn't sloop; the caller prints manual chaining instructions.
var errNotifyOccupied = errors.New("codex notify slot already in use")

// notifyCommand is the argv sloop registers as a tool's notify program.
func notifyCommand(tool string) []string {
	return []string{appName, "hooks", "notify", tool}
}

// installCodexHooks claims codex's notify slot for sloop when it is free.
func installCodexHooks(path string, _ adapter.HooksSpec) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	var doc map[string]any
	if len(raw) > 0 {
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return false, fmt.Errorf("%s is not valid TOML: %w", path, err)
		}
	}
	want := notifyCommand("codex")
	if cur, ok := doc["notify"]; ok {
		if cmd, ok := cur.([]any); ok && notifyEqual(cmd, want) {
			return false, nil
		}
		return false, errNotifyOccupied
	}
	// Build the notify line manually (toml.Marshal would use single quotes).
	line := buildNotifyLine(want)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	out := append(line, raw...)
	return true, os.WriteFile(path, out, 0o644)
}

// buildNotifyLine formats the notify array as a TOML line with double quotes.
func buildNotifyLine(cmd []string) []byte {
	parts := make([]string, len(cmd))
	for i, s := range cmd {
		parts[i] = fmt.Sprintf("%q", s)
	}
	return []byte(fmt.Sprintf("notify = [%s]\n", strings.Join(parts, ", ")))
}

// notifyEqual compares a decoded TOML notify array against sloop's argv.
func notifyEqual(got []any, want []string) bool {
	gs := make([]string, 0, len(got))
	for _, v := range got {
		s, ok := v.(string)
		if !ok {
			return false
		}
		gs = append(gs, s)
	}
	return reflect.DeepEqual(gs, want)
}

// codexChainHint is printed when the notify slot is occupied.
func codexChainHint(path string) string {
	return strings.Join([]string{
		"codex's single notify slot in " + path + " is already in use; sloop won't touch it.",
		"To chain both, point notify at a small script that runs sloop first:",
		"  #!/bin/sh",
		"  " + appName + " hooks notify codex \"$1\"",
		"  exec <your current notify program> \"$1\"",
	}, "\n")
}

// codexNotifyInstalled reports whether the given TOML bytes contain sloop's
// notify command. Used by hooksInstalledFor to keep TOML parsing in this file.
func codexNotifyInstalled(b []byte) bool {
	var doc map[string]any
	if toml.Unmarshal(b, &doc) != nil {
		return false
	}
	cmd, ok := doc["notify"].([]any)
	return ok && notifyEqual(cmd, notifyCommand("codex"))
}
