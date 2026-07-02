package adapter

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/stroops/sloop/internal/config"
	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

type ContextSpec struct {
	Mode string `yaml:"mode"` // "pointer" | "native"
	File string `yaml:"file"` // pointer mode only
}

type SkillsSpec struct {
	Target string `yaml:"target"` // dir to link .sloop/skills into; empty = none
}

// EventSpec locates one provider event: its name plus an optional matcher
// discriminating within it (e.g. copilot's notification types). Workflow
// hooks (v0.2.0) will reuse this type; fields with no consumer yet
// (cadence, decision) stay out until that build — see docs/design/hooks.md.
type EventSpec struct {
	Event   string `yaml:"event"`
	Matcher string `yaml:"matcher"`
}

// HookEvents maps each sloop status to the tool's own lifecycle event.
// A zero EventSpec means the tool has no signal for that state.
type HookEvents struct {
	Working EventSpec `yaml:"working"` // tool started working
	Waiting EventSpec `yaml:"waiting"` // tool is blocked on the user
	Idle    EventSpec `yaml:"idle"`    // tool finished the turn
}

// HooksSpec captures how a tool exposes lifecycle hooks for precise status. It
// keeps per-provider hook knowledge in the manifest (the single provider-aware
// source), not hardcoded in commands.
type HooksSpec struct {
	Config  string     `yaml:"config"`  // config file (repo-relative or ~/…)
	Install string     `yaml:"install"` // installer strategy: "settings-json" | "" (manual)
	Docs    string     `yaml:"docs"`
	Notes   string     `yaml:"notes"`
	Events  HookEvents `yaml:"events"`
}

// StatusLinePayload maps sloop's display fields to dotted paths inside the
// JSON payload a tool pipes to its statusline command. Context usage comes
// either as a ready percentage (ContextPct) or as token paths to sum
// (ContextUsed) against a window size (ContextSize) — whichever the tool
// sends. Rate-limit usage is similarly two conventions: a ready "used"
// percentage (RateLimitUsedPct) or a "remaining" fraction to invert
// (RateLimitRemainingFrac); its reset is either an absolute epoch-seconds
// timestamp (RateLimitResetsAt) or a relative seconds-until-reset
// (RateLimitResetsIn) — a tool sends exactly one of each pair.
type StatusLinePayload struct {
	Model       string   `yaml:"model"`        // e.g. model.display_name
	Cwd         string   `yaml:"cwd"`          // working directory of the session
	ContextPct  string   `yaml:"context_pct"`  // 0–100 number, if the tool sends one
	ContextUsed []string `yaml:"context_used"` // token counts to sum
	ContextSize string   `yaml:"context_size"` // window size the sum is divided by
	State       string   `yaml:"state"`        // tool's own agent state, if present

	// Rate-limit usage: something no custom statusline script typically shows,
	// so sloop surfaces it as new information rather than repeating the tool's
	// own footer. Absent (all "") on tools that report none.
	RateLimitUsedPct       string `yaml:"rate_limit_used_pct"`       // 0–100, e.g. Claude's rate_limits.five_hour.used_percentage
	RateLimitRemainingFrac string `yaml:"rate_limit_remaining_frac"` // 0–1, e.g. agy's quota.gemini-5h.remaining_fraction
	RateLimitResetsAt      string `yaml:"rate_limit_resets_at"`      // unix epoch seconds, e.g. Claude's rate_limits.five_hour.resets_at
	RateLimitResetsIn      string `yaml:"rate_limit_resets_in"`      // seconds from now, e.g. agy's quota.gemini-5h.reset_in_seconds
}

// StatusLineSpec captures how a tool's built-in statusline mechanism works, so
// sloop can register a feed there (`sloop statusline feed <tool>`) and enrich
// the fleet view with model/context info the tool itself reports. Like hooks,
// this is provider-respecting: the tool invokes sloop through its own
// documented mechanism.
type StatusLineSpec struct {
	Config  string            `yaml:"config"`  // settings file holding the statusLine key (~/…)
	Install string            `yaml:"install"` // installer strategy: "settings-json" | "" (none)
	Docs    string            `yaml:"docs"`
	Payload StatusLinePayload `yaml:"payload"`
	// States maps the tool's payload state values to sloop's waiting/working/idle,
	// for tools whose statusline payload doubles as a status signal (e.g. agy,
	// which has no lifecycle hooks). Unmapped values are ignored.
	States map[string]string `yaml:"states"`
}

// HeuristicsSpec defines fallback text-matching rules to classify agent status
// when native hooks aren't available. In the future, this could be extended
// with LLM prompts for deeper "AI awareness" of the session state.
type HeuristicsSpec struct {
	Working []string `yaml:"working"`
	Waiting []string `yaml:"waiting"`
	// Model is a regexp (capture group 1 = model name) matched against the
	// visible pane text, for tools whose TUI shows the current model but expose
	// no statusline/hook to report it. Live but brittle by nature: when it
	// stops matching (UI redraw, version change), the status bar falls back to
	// the last known model.
	Model string `yaml:"model"`
}

// RunSpec declares how `sloop run` launches a tool with an optional model and
// reasoning effort. Every field is optional: an empty flag means the CLI has no
// such knob, and sloop errors clearly if the user asks for it. sloop never
// validates the model; it forwards the string and lets the CLI accept/reject it.
type RunSpec struct {
	// Vendor is the model vendor this CLI natively serves (e.g. "anthropic").
	Vendor string `yaml:"vendor"`
	// DefaultFor lists vendors this CLI is the canonical launcher for, so a bare
	// model (`sloop run opus`) resolves to it.
	DefaultFor []string `yaml:"default_for"`
	// ModelFlag is how the CLI selects a model (e.g. "--model"); "" = unsupported.
	ModelFlag string `yaml:"model_flag"`
	// EffortFlag + EffortValues express reasoning effort: low|medium|high mapped
	// to the CLI's own token. "" flag = the CLI has no effort knob.
	EffortFlag   string            `yaml:"effort_flag"`
	EffortValues map[string]string `yaml:"effort_values"`
	// Models are the model aliases this CLI serves (completion + bare-model
	// resolution). Not exhaustive; full API ids still work via forward.
	Models []string `yaml:"models"`
	// Prompt is how an initial interactive task (`sloop run … -t "…"`) is passed:
	// "positional" appends it as a bare arg, a flag string passes `<flag> <task>`,
	// "" = the CLI has no initial-task support via sloop.
	Prompt string `yaml:"prompt"`
	// ResumeFlag continues the most recent conversation (used by `sloop restore
	// --resume`); "" = the CLI can't resume.
	ResumeFlag string `yaml:"resume_flag"`
}

type Manifest struct {
	Name string `yaml:"name"`
	// Detect is the binary name used to detect installation; Launch is the
	// binary to run (often the same).
	Detect  string      `yaml:"detect"`
	Launch  string      `yaml:"launch"`
	Context ContextSpec `yaml:"context"`
	Skills  SkillsSpec  `yaml:"skills"`
	Hooks   HooksSpec   `yaml:"hooks"`
	// Run declares model/effort launch knobs for `sloop run`.
	Run RunSpec `yaml:"run"`
	// StatusLine declares the tool's built-in statusline mechanism, feeding
	// model/context info (and, for hook-less tools, status) into the fleet view.
	StatusLine StatusLineSpec `yaml:"statusline"`
	// Fallback text-parsing rules for status (paving the way for AI-driven awareness)
	Heuristics HeuristicsSpec `yaml:"heuristics"`
	// Scaffold lists the tool's standard project dirs that `sloop init --scaffold`
	// can create (e.g. .claude/skills, .cursor/rules), so users start from the
	// provider's expected layout instead of an ad-hoc one.
	Scaffold []string `yaml:"scaffold"`
	// Readiness declares extra best-practice checks for `sloop check`, sourced
	// from the provider's own docs, so the checklist deepens by editing data.
	Readiness ReadinessSpec `yaml:"readiness"`
	// Account declares how the tool selects a separate account/config dir, so
	// `sloop profile add --config-dir` works without the user knowing the env var.
	Account AccountSpec `yaml:"account"`
}

// AccountSpec declares how a tool points at an alternate account/config
// directory, so `sloop profile add --config-dir <dir>` can target it without the
// user having to know the tool's environment variable. Only tools with a
// non-empty ConfigDirEnv support `--config-dir`.
type AccountSpec struct {
	// ConfigDirEnv is the environment variable that selects an alternate config/
	// account directory (e.g. CLAUDE_CONFIG_DIR). Its presence is what makes a
	// tool eligible for `--config-dir`.
	ConfigDirEnv string `yaml:"config_dir_env"`
	// DefaultDir is the tool's standard config dir (e.g. ~/.claude), the source
	// to optionally share tooling/history from when wiring up a second account.
	DefaultDir string `yaml:"default_dir"`
	// Share lists subpaths under DefaultDir that are safe to symlink into a new
	// account dir (tooling/config, the same across accounts). Never credentials.
	Share []string `yaml:"share"`
	// ShareState lists subpaths carrying conversation/session state; sharing them
	// lets a second account resume the first's sessions (opt-in, off by default).
	ShareState []string `yaml:"share_state"`
}

// ReadinessSpec is a provider's best-practice checklist for `sloop check`. Docs
// is the source these criteria come from (the provider's own guidance), so each
// check is attributable, not sloop's opinion.
type ReadinessSpec struct {
	Docs   string           `yaml:"docs"`
	Checks []ReadinessCheck `yaml:"checks"`
}

// ReadinessCheck is one declarative, filesystem-detectable best practice. Kind is
// "file-exists" | "dir-exists" | "git-tracked"; Path is repo-relative. Optional
// checks surface as advisory info (not counted as a gap) so power-user features
// don't nag a basic setup.
type ReadinessCheck struct {
	ID       string `yaml:"id"`
	Label    string `yaml:"label"`
	Kind     string `yaml:"kind"`
	Path     string `yaml:"path"`
	Fix      string `yaml:"fix"`
	Optional bool   `yaml:"optional"`
}

func LoadBuiltin() (map[string]Manifest, error) {
	out := map[string]Manifest{}
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		// embed.FS always uses forward-slash paths regardless of OS, so use
		// path.Join (not filepath.Join, which would yield "builtin\name" on Windows).
		b, err := fs.ReadFile(builtinFS, path.Join("builtin", e.Name()))
		if err != nil {
			return nil, err
		}
		var m Manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(e.Name(), ".yaml")
		out[key] = m
	}
	return out, nil
}

// Load returns built-in manifests overlaid with any user manifests in
// ~/.sloop/adapters/*.yaml (same key overrides the built-in).
func Load() (map[string]Manifest, error) {
	out, err := LoadBuiltin()
	if err != nil {
		return nil, err
	}
	dir, err := config.UserAdaptersDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var m Manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(e.Name(), ".yaml")
		out[key] = m
	}
	return out, nil
}
