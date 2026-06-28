# Design: profiles & named instances — run a second agent (or a second account) of the same provider

**Date:** 2026-06-28
**Status:** Approved design, ready for implementation plan
**Scope:** Global profiles, named instances, env injection, `sloop profile` command

## Problem

Today a session is named `<workspace>__<tool>` (`internal/tmux/tmux.go:84`), and
`tmux.Runner.Launch` re-attaches when that session already exists
(`internal/tmux/tmux.go:208`). Two consequences:

1. **You cannot run a second agent of the same provider in one workspace.**
   `sloop run claude` a second time just re-attaches the existing
   `<ws>__claude` — there is no place in the name to distinguish a second
   instance.
2. **You cannot run a different account of the same provider.** sloop launches
   the real binary `m.Launch` (e.g. `claude`) directly via tmux `new-session`,
   *not* through a shell, so a user's shell alias (`claude-sec`) does not apply,
   and there is no way to inject the env that selects another account
   (`CLAUDE_CONFIG_DIR=~/.claude-sec`). `runner.Spec` has no `Env` field.

Both gaps reduce to two missing primitives — a per-instance name suffix, and
env injection at launch — plus declarative sugar so a reused alternate account
is one word.

## Non-goals (deliberately deferred)

- **Per-project profiles / project-over-global merge.** Profiles live in the
  global config only. An alternate account is a personal thing reused across
  every repo; per-project re-declaration is YAGNI.
- **`model`/`effort`/extra-args inside a profile.** v1 profiles carry `tool` +
  `env` only. Model/effort still pass through at the call site
  (`sloop run @sec -m opus`). The schema leaves room to add them later.
- **Validating env values** (e.g. checking the config dir exists). Values are
  expanded (`~`, `$VAR`) and forwarded as-is, matching how `--model` is
  forwarded un-validated.
- **Changing the default re-attach behavior.** `sloop run claude` with no
  instance still means "jump to my claude." A fresh instance is always explicit
  (`@`, `tool@instance`, `--name`, or `--new`).

## Token grammar

One positional grammar covers both profiles and ad-hoc instances. `@` always
introduces a *name*; what is left of it (if anything) is the tool.

| Token | Meaning | Session |
|---|---|---|
| `claude` | tool, default instance (unchanged) | `ws__claude` |
| `@sec` | left of `@` empty → **profile** `sec` | `ws__claude__sec` |
| `claude@b` | left is a tool → ad-hoc **instance** `b`, default account | `ws__claude__b` |

Resolution rule for a token containing `@`, split once into `left@right`:

- `left == ""` → look up **profile** `right` in global config. Profile supplies
  `tool` (required) and `env`. Instance name defaults to the profile key
  (`right`), overridable by `--name`.
- `left` is a known tool/alias → ad-hoc **instance** `right` on that tool, no
  env. Equivalent to `sloop run <left> --name <right>`.
- `left` is neither empty nor a known tool → error:
  `unknown tool %q in %q (use @<profile> or <tool>@<instance>)`.

Tokens without `@` resolve exactly as today (tool key → binary alias → model
alias).

## Flags on `sloop run`

- `-n, --name <instance>` — explicit instance suffix. Sanitized; rejected if it
  contains `__` (would break the parser). Overrides a profile key.
- `--env KEY=VAL` (repeatable, `StringArray`) — ad-hoc env injection without a
  profile. Merged after profile env (call-site wins on key clash).
- `-N, --new` — when no explicit name/instance is given, pick the first free
  instance slot instead of re-attaching: try `ws__tool`, then `ws__tool__2`,
  `__3`, … and create the first that does not exist. With an explicit name it is
  a no-op (the name already makes it distinct).

## Profile schema (global `~/.sloop/config.yaml`)

```yaml
profiles:
  sec:
    tool: claude                        # required: a manifest key (or alias)
    env:
      CLAUDE_CONFIG_DIR: ~/.claude-sec   # ~ and $VAR expanded at resolve time
```

Add to `config.Global`:

```go
type Profile struct {
    Tool string            `yaml:"tool"`
    Env  map[string]string `yaml:"env,omitempty"`
}
// Global gains:
Profiles map[string]Profile `yaml:"profiles,omitempty"`
```

`LoadGlobal()` already tolerates missing keys, so old configs load with
`Profiles == nil`.

## Session naming

```go
// InstanceName returns ws__tool for instance=="", else ws__tool__<instance>.
func InstanceName(workspace, tool, instance string) string
```

`SessionName(ws, tool)` stays for the default/legacy call sites; `InstanceName`
is the superset used by `run`. The session DB already has a `Profile` column
(`internal/session/store.go:152`) that has never carried a real value — it now
records the instance name.

## Tool-aware name parser (the one careful bit)

Two sites *extract the tool* from a session name today, both in the `commands`
package and both using `strings.LastIndex(name, "__")`:

- `internal/cli/commands/ps.go:87` (`fleetRows`)
- `internal/cli/commands/statusline.go:36` (`renderStatusline`)

A three-segment `ws__tool__instance` breaks `LastIndex` (it would read the
instance as the tool). Replace both with one shared, manifest-aware helper:

```go
// splitSession parses a sloop session name into workspace, tool, instance.
// Manifest-aware so the tool is identified by key, not by position:
//   - if the last "__" segment is a known tool → tool=last, instance="" (legacy)
//   - else if the second-to-last is a known tool → tool=that, instance=last
//   - else fall back to LastIndex (unknown tool, instance="")
func splitSession(name string, manifests map[string]adapter.Manifest) (ws, tool, instance string)
```

This is backward compatible (two-segment names parse exactly as before) and
robust to a workspace name that itself contains `__`, because the tool is found
by key rather than by counting separators. (`renderFleetBadge` only *detects*
sloop sessions via `LastIndex(name, "__") < 0`, which still holds for
three-segment names, so it is left unchanged.)

## Env injection

Add `Env map[string]string` to `runner.Spec`. Two runners consume it:

- **tmux** (`tmux.Runner.Launch`): prefix the launched command with the `env`
  binary so it works on every tmux version (no dependency on `new-session -e`):

  ```
  new-session -d -s <session> -c <dir> env K1=V1 K2=V2 <launch> <args…>
  ```

  `env` keys are emitted in sorted order for deterministic output/tests; nil/empty
  env produces no prefix (byte-identical to today).
- **exec** (`runner.ExecRunner`, no tmux): `cmd.Env = append(os.Environ(), …)`.

Values are expanded before launch: a leading `~/` → `$HOME/`, and `$VAR`/
`${VAR}` via `os.Expand` against the current environment.

## Fleet display

The instance shows next to the tool so two claudes in one repo are
distinguishable: `claude·sec`, `claude·b`, `claude·2`. `FleetRow` gains an
`Instance` field set by `splitSession`; `displayTool` renders `tool` when
instance is empty, else `tool·instance`. `ps`, the picker, the status line, and
the waiting badge all read through that one display path.

## `sloop profile` command

A small group that edits the global config (hand-editing the YAML stays valid):

```
sloop profile add <name> --tool <tool> [--env KEY=VAL …]   # write/overwrite
sloop profile ls                                            # name · tool · env keys
sloop profile rm <name>                                     # delete
```

`add` validates that `--tool` resolves to a manifest key (reusing
`toolKeyFor`) and writes via `SaveGlobal`. `ls` with no profiles prints a hint
pointing at `profile add`.

## Error handling

- Unknown profile → `unknown profile %q (see: sloop profile ls)`.
- Profile's `tool` not a manifest key → `profile %q uses unknown tool %q`.
- `--name` containing `__` → `instance name %q cannot contain "__"`.
- `--env` not `KEY=VAL` → `--env must be KEY=VAL, got %q`.
- `profile rm` of a missing name → `no profile %q to remove`.

## Testing

Pure, table-driven units (no tmux/network), matching the existing run tests:

- `splitSession`: 2-seg legacy, 3-seg instance, unknown tool fallback,
  workspace-containing-`__`.
- token grammar: `@sec` → profile, `claude@b` → instance, `claude` → plain,
  `bad@x` → error, profile-key default vs `--name` override.
- `InstanceName`: empty vs non-empty instance; sanitize.
- env: `--env` parse + merge order (profile then `--env`), `~`/`$VAR` expansion,
  sorted `env` prefix in the tmux create args, empty env → no prefix.
- `--new`: first free slot given a set of existing session names.
- profile config: round-trip add/ls/rm through `Global`, `add` rejects unknown
  tool.

Manual (cannot be unit-tested — needs a live tmux and two real accounts):
`sloop run @sec` actually launches claude under `~/.claude-sec`; two instances
coexist in `sloop ps` as `claude·sec` / `claude`; `--new` spins `claude·2`.
