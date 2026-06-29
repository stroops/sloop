# Adapters: the provider-capability contract

Sloop is multi-provider. **All per-provider knowledge lives in one place: the adapter manifest.**
Every provider-aware feature reads it; no feature hardcodes a tool name.

> **The rule:** to support a new AI CLI, add one manifest. Never special-case a tool in code.

## Where manifests live

- **Built-in:** `internal/adapter/builtin/<tool>.yaml`, embedded in the binary (`go:embed`).
- **User overrides / new tools:** `~/.sloop/adapters/<tool>.yaml`. A file with the same key
  overrides the built-in; a new key adds a tool. Loaded by `adapter.Load()`
  (`internal/adapter/adapter.go`).

Manifests are sloop's *provider knowledge / plugin layer*, **not** user config. Users normally
never touch them (enabled tools are chosen in `.sloop/config.yaml`; see `docs/reference/CONFIG.md`).

## Manifest schema

```yaml
name: Claude Code              # display name
detect: claude                 # binary name used to detect installation
launch: claude                 # binary to launch

context:                       # how AGENTS.md context reaches the tool
  mode: pointer                #   "pointer" (write a pointer file) | "native" (reads AGENTS.md itself)
  file: CLAUDE.md              #   pointer file name (pointer mode only)

skills:                        # how .sloop/skills is delivered
  target: .claude/skills       #   dir to symlink skills into; "" = tool has no skills dir

hooks:                         # status hooks for `sloop ps` (see docs/guide/USAGE.md)
  config: .claude/settings.local.json   # where the tool configures hooks (repo-relative or ~/…)
  install: settings-json                # installer strategy: "settings-json" | "" (manual)
  docs: https://…                       # link to the tool's hook docs
  notes: ""                             # caveats for manual setup
  events:                               # the tool's event name for each sloop state
    working: UserPromptSubmit           #   "" if the tool can't signal that state
    waiting: Notification
    idle: Stop

run:                           # `sloop run` model/effort knobs (all optional); see docs/design/run.md
  vendor: anthropic            #   model vendor this CLI natively serves
  default_for: [anthropic]     #   vendors this CLI is the home launcher for (bare `sloop run opus`)
  model_flag: "--model"        #   how to pass a model; "" = no model selection
  effort_flag: ""              #   how to pass reasoning effort; "" = unsupported
  effort_values: {}            #   low|medium|high → the CLI's own token (e.g. -c model_reasoning_effort=high)
  models: [opus, sonnet]       #   aliases for completion + bare-model resolution (forwarded as-is)
  prompt: positional           #   how `run -t "task"` is passed: "positional" | a flag | "" (none)

account:                       # OPTIONAL: how a 2nd account is selected (powers `profile add --config-dir`)
  config_dir_env: CLAUDE_CONFIG_DIR   #   env var that points the tool at an alternate config dir
  default_dir: ~/.claude              #   the tool's standard config dir (source to share from)
  share: [plugins, agents, commands, skills, CLAUDE.md]   # safe to symlink into the 2nd account (tooling)
  share_state: [projects, todos]      #   conversation/session state; opt-in, lets a 2nd account resume

heuristics:                    # OPTIONAL fallback status markers (see below); usually omit
  waiting: ["shall I apply"]   #   tool-specific phrasing the cross-tool defaults miss
  working: ["indexing repo"]

readiness:                     # OPTIONAL extra `sloop check` best practices, sourced from the
  docs: https://…              #   provider's own guidance (cited so it's not sloop's opinion)
  checks:                      #   only filesystem-detectable practices; behavioral tips don't belong
    - id: subagents
      label: "Subagents folder (.claude/agents)"
      kind: dir-exists         #   "file-exists" | "dir-exists" | "git-tracked"
      path: .claude/agents     #   repo-relative
      fix: "sloop init --scaffold"
      optional: true           #   missing → advisory info (•), not a counted gap (✗)
```

## Which features are manifest-driven

| Feature | Reads | Field |
|---|---|---|
| detect / `sloop tools` | `detect.Tools` | `detect` |
| `sloop run` launch | `run.go` | `launch` |
| `sloop run` model/effort | `run.go` (`planLaunch`/`buildRunArgs`) | `run.*` |
| `sloop sync` context | `sync` | `context.mode/file` |
| skills symlink | `sync` | `skills.target` |
| `sloop hooks` + status | `hooks.go` | `hooks.*` |
| `sloop ps` status fallback | `tmux.ClassifyStatus` | `heuristics.*` (additive; see below) |
| `sloop check` readiness | `check.go` | `readiness.*` (optional; provider-sourced) |
| `sloop profile add --config-dir` | `account.go` | `account.*` (optional; 2nd-account dir + sharing). `.credentials.json` is never shared. |
| shell completion | `completion.go` | manifest keys |

The runtime view of all of this is **`sloop tools`** (capability matrix:
`KEY NAME INSTALLED CONTEXT SKILLS HOOKS`).

## Hook install strategies

- `settings-json`: merge `events → "sloop hooks emit <state>"` into a JSON settings file
  (`mergeSettingsHooks`/`installSettingsHooks`). Used by **claude** and **gemini** (identical nested
  `hooks[event] = [{hooks:[{type,command}]}]` shape). Written idempotently, never clobbering keys.
- `cursor-json`: merge into `.cursor/hooks.json`, Cursor's flatter shape
  (`{version, hooks[event]=[{command}]}`; `mergeCursorHooks`/`installCursorHooks`). Used by **cursor**.
  Note: Cursor has no clean "blocked on user" event (`beforeShellExecution` fires on every command),
  so its manifest leaves `waiting: ""` and the pane heuristic covers waiting.
- `""` (manual): no safe auto-writer yet; `sloop hooks print <tool>` shows the exact
  event→command wiring and `sloop hooks list` marks it `print+paste`. **Copilot/Codex** are here:
  Copilot uses a single `notification` event with matchers (`permission_prompt`/`agent_idle`/…) plus
  per-OS `bash`/`powershell` keys; Codex is TOML with one `notify` program for all events (would need
  payload routing). Both need a matcher-aware model before auto-install.

Strategies dispatch through `hookInstaller(strategy)`: add a `case` there + an `install…Hooks`
writer to teach a new config format.

## Status heuristics (fallback only)

`sloop ps` classifies each session as waiting / working / idle. The **precise** signal is the
`hooks` above (the tool tells sloop its state). Heuristics are the **fallback** when hooks aren't
installed: a non-invasive read of the tool's own pane text.

Most prompt conventions (`(y/n)`, `press enter`, `esc to interrupt`, spinners, `tokens`) are
**cross-tool defaults** baked in once (`defaultWaitingMarkers`/`defaultWorkingMarkers` in
`internal/tmux/status.go`) and applied to **every** manifest. They are terminal conventions, not
per-provider knowledge, so they live in code, not duplicated into each YAML.

A manifest's optional `heuristics` block is **additive on top of** the defaults; add markers only
for phrasing a specific tool uses that the defaults genuinely miss. If a tool needs none (most don't,
e.g. claude), omit the block. Don't re-list default markers. If you want precision, install the
tool's hooks rather than growing heuristics.

## Adding a new CLI (checklist)

1. Drop `internal/adapter/builtin/<tool>.yaml` (or `~/.sloop/adapters/<tool>.yaml`) with the schema
   above. Set `hooks.events`/`install` from the tool's hook docs.
2. Verify with `sloop tools` (capabilities) and `sloop hooks print <tool>`.
3. If its hooks use the settings.json shape, `install: settings-json` makes it auto-install for free.
   Otherwise leave `install: ""` and it's print-and-paste until a strategy is added.

No Go code changes are required for a new provider unless it needs a brand-new hook install strategy.
