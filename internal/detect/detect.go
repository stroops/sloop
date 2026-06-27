package detect

import (
	"context"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/stroops/sloop/internal/adapter"
)

type ToolStatus struct {
	Key       string
	Name      string
	Binary    string
	Installed bool
	Version   string
}

func Tools(manifests map[string]adapter.Manifest) []ToolStatus {
	var out []ToolStatus
	for key, m := range manifests {
		s := ToolStatus{Key: key, Name: m.Name, Binary: m.Detect}
		if _, err := exec.LookPath(m.Detect); err == nil {
			s.Installed = true
			s.Version = bestEffortVersion(m.Detect)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func InstalledKeys(manifests map[string]adapter.Manifest) []string {
	var keys []string
	for key, m := range manifests {
		if _, err := exec.LookPath(m.Detect); err == nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

type TmuxStatus struct {
	Installed bool
	Version   string
}

func Tmux() TmuxStatus {
	if _, err := exec.LookPath("tmux"); err != nil {
		return TmuxStatus{}
	}
	return TmuxStatus{Installed: true, Version: bestEffortVersion2("tmux", "-V")}
}

// bestEffortVersion runs `<bin> --version` with a short timeout, returning the
// first line of output or "" on any failure.
func bestEffortVersion(bin string) string {
	return bestEffortVersion2(bin, "--version")
}

func bestEffortVersion2(bin, flag string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, flag).Output()
	if err != nil {
		return ""
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return strings.TrimSpace(line)
}
