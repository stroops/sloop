package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/runner"
	"github.com/stroops/sloop/internal/tmux"
)

// instanceResolution is the outcome of interpreting a `sloop run` target for
// profiles / named instances: which tool token to plan, the instance suffix
// (""=default session), and env to inject.
type instanceResolution struct {
	target   string
	instance string
	env      map[string]string
}

// resolveInstance interprets the `@profile` / `tool@instance` token grammar and
// folds in --name and --env. `@` always introduces a name: an empty left side
// looks up a profile (supplying tool + env); a tool on the left is an ad-hoc
// instance. --env merges over profile env (call-site wins); --name overrides the
// instance.
func resolveInstance(target, nameFlag string, envFlags []string, profiles map[string]config.Profile, manifests map[string]adapter.Manifest) (instanceResolution, error) {
	res := instanceResolution{target: target}
	if left, right, ok := strings.Cut(target, "@"); ok {
		if right == "" {
			return res, fmt.Errorf("missing name after '@' in %q", target)
		}
		if left == "" {
			prof, ok := profiles[right]
			if !ok {
				return res, fmt.Errorf("unknown profile %q (see: sloop profile ls)", right)
			}
			if _, ok := toolKeyFor(prof.Tool, manifests); !ok {
				return res, fmt.Errorf("profile %q uses unknown tool %q", right, prof.Tool)
			}
			res.target = prof.Tool
			res.instance = right
			res.env = expandEnvMap(prof.Env)
		} else {
			if _, ok := toolKeyFor(left, manifests); !ok {
				return res, fmt.Errorf("unknown tool %q in %q (use @<profile> or <tool>@<instance>)", left, target)
			}
			res.target = left
			res.instance = right
		}
	}
	if len(envFlags) > 0 {
		ev, err := parseEnvFlags(envFlags)
		if err != nil {
			return res, err
		}
		res.env = mergeEnv(res.env, expandEnvMap(ev))
	}
	if nameFlag != "" {
		res.instance = nameFlag
	}
	if strings.Contains(res.instance, "__") {
		return res, fmt.Errorf(`instance name %q cannot contain "__"`, res.instance)
	}
	return res, nil
}

// parseEnvFlags turns `KEY=VAL` flags into a map; the first `=` splits, so the
// value may itself contain `=`.
func parseEnvFlags(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(flags))
	for _, f := range flags {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--env must be KEY=VAL, got %q", f)
		}
		out[k] = v
	}
	return out, nil
}

// expandEnvValue expands a leading `~/` to the home dir and any `$VAR`/`${VAR}`
// against the current environment, so profile env values can be written the way
// a user would type them in a shell.
func expandEnvValue(v string) string {
	if strings.HasPrefix(v, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			v = home + v[1:]
		}
	}
	return os.Expand(v, os.Getenv)
}

func expandEnvMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = expandEnvValue(v)
	}
	return out
}

// mergeEnv overlays over onto base (over wins on key clash), returning a usable
// map even when either side is nil.
func mergeEnv(base, over map[string]string) map[string]string {
	if len(base) == 0 {
		return over
	}
	for k, v := range over {
		base[k] = v
	}
	return base
}

// nextFreeInstance returns the first instance suffix whose session name is not
// already taken: "" (ws__tool), then "2" (ws__tool__2), "3", …; so `--new`
// spins a fresh agent instead of re-attaching the existing one.
func nextFreeInstance(ws, tool string, sessions []tmux.Session) string {
	taken := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		taken[s.Name] = true
	}
	if !taken[tmux.InstanceName(ws, tool, "")] {
		return ""
	}
	for n := 2; ; n++ {
		inst := strconv.Itoa(n)
		if !taken[tmux.InstanceName(ws, tool, inst)] {
			return inst
		}
	}
}

// selectRunnerInstance is selectRunner with an instance suffix on the session
// name (tmux when available, else exec).
func selectRunnerInstance(ws, tool, instance string) runner.Runner {
	if tmux.Available() {
		return tmux.Runner{Session: tmux.InstanceName(ws, tool, instance)}
	}
	return runner.ExecRunner{}
}
