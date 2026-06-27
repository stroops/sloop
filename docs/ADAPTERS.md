# Adapters — the provider-capability contract

Sloop is multi-provider. **All per-provider knowledge lives in one place: the adapter manifest.**
Every provider-aware feature reads it; no feature hardcodes a tool name.

> **The rule:** to support a new AI CLI, add one manifest. Never special-case a tool in code.

## Where manifests live

- **Built-in:** `internal/adapter/builtin/<tool>.yaml`, embedded in the binary (`go:embed`).
- **User overrides / new tools:** `~/.sloop/adapters/<tool>.yaml`. A file with the same key
  overrides the built-in; a new key adds a tool. Loaded by `adapter.Load()`
  (`internal/adapter/adapter.go`).

Manifests are sloop's *provider knowledge / plugin layer* — **not** user config. Users normally
never touch them (enabled tools are chosen in `.sloop/config.yaml`; see `docs/CONFIG.md`).

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

hooks:                         # status hooks for `sloop ps` (see docs/USAGE.md)
  config: .claude/settings.local.json   # where the tool configures hooks (repo-relative or ~/…)
  install: settings-json                # installer strategy: "settings-json" | "" (manual)
  docs: https://…                       # link to the tool's hook docs
  notes: ""                             # caveats for manual setup
  events:                               # the tool's event name for each sloop state
    working: UserPromptSubmit           #   "" if the tool can't signal that state
    waiting: Notification
    idle: Stop
```

## Which features are manifest-driven

| Feature | Reads | Field |
|---|---|---|
| detect / `sloop tools` | `detect.Tools` | `detect` |
| `sloop run` launch | `run.go` | `launch` |
| `sloop sync` context | `sync` | `context.mode/file` |
| skills symlink | `sync` | `skills.target` |
| `sloop hooks` + status | `hooks.go` | `hooks.*` |
| shell completion | `completion.go` | manifest keys |

The runtime view of all of this is **`sloop tools`** (capability matrix:
`KEY NAME INSTALLED CONTEXT SKILLS HOOKS`).

## Hook install strategies

- `settings-json` — merge `events → "sloop hook <state>"` into a JSON settings file
  (`mergeSettingsHooks`/`installSettingsHooks`). Used by **claude** and **gemini** (identical shape).
  `sloop hooks install <tool>` writes it idempotently, never clobbering existing keys.
- `""` (manual) — no safe auto-writer yet; `sloop hooks print <tool>` shows the exact
  event→command wiring and `sloop hooks list` marks it `print+paste`. Cursor/Copilot/Codex are here
  (different config formats; Codex needs a `notify`-payload mode and is TOML, which we don't parse).

## Adding a new CLI (checklist)

1. Drop `internal/adapter/builtin/<tool>.yaml` (or `~/.sloop/adapters/<tool>.yaml`) with the schema
   above. Set `hooks.events`/`install` from the tool's hook docs.
2. Verify with `sloop tools` (capabilities) and `sloop hooks print <tool>`.
3. If its hooks use the settings.json shape, `install: settings-json` makes it auto-install for free.
   Otherwise leave `install: ""` and it's print-and-paste until a strategy is added.

No Go code changes are required for a new provider unless it needs a brand-new hook install strategy.
