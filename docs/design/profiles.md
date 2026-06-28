# Design: profiles & named instances — run a second agent (or a second account) of the same provider

**Date:** 2026-06-28
**Status:** Shipped
**Scope:** Global profiles, named instances, env injection, `sloop profile` command

## Problem

A session is named `<workspace>__<tool>`, and launching re-attaches when that
session already exists. Two consequences:

1. **You cannot run a second agent of the same provider in one workspace.**
   `sloop run claude` a second time just re-attaches the existing `<ws>__claude`
   — there is no place in the name to distinguish a second instance.
2. **You cannot run a different account of the same provider.** sloop launches
   the real binary (e.g. `claude`) directly, *not* through a shell, so a user's
   shell alias (`claude-sec`) does not apply, and there is nowhere to inject the
   env that selects another account (`CLAUDE_CONFIG_DIR=~/.claude-sec`).

Both gaps reduce to two missing primitives — a per-instance name suffix, and env
injection at launch — plus declarative sugar so a reused alternate account is one
word.

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
  (`@profile`, `tool@instance`, `--name`, or `--new`).

## Token grammar

One positional grammar covers both profiles and ad-hoc instances. `@` always
introduces a *name*; what is left of it (if anything) is the tool.

| Token | Meaning | Session |
|---|---|---|
| `claude` | tool, default instance (unchanged) | `ws__claude` |
| `@sec` | left of `@` empty → **profile** `sec` | `ws__claude__sec` |
| `claude@b` | left is a tool → ad-hoc **instance** `b`, default account | `ws__claude__b` |

For a token containing `@`, split once into `left@right`:

- `left` empty → look up **profile** `right` in the global config. The profile
  supplies the tool (required) and env. The instance name defaults to the
  profile key (`right`), overridable by `--name`.
- `left` is a known tool/alias → ad-hoc **instance** `right` on that tool, no
  env — equivalent to `sloop run <left> --name <right>`.
- `left` is neither empty nor a known tool → error (it is neither a profile nor
  a known tool).

Tokens without `@` resolve exactly as today (tool key → binary alias → model
alias).

## Flags on `sloop run`

- `-n, --name <instance>` — explicit instance suffix. Sanitized; rejected if it
  contains `__` (the parser uses `__` as the separator). Overrides a profile key.
- `--env KEY=VAL` (repeatable) — ad-hoc env injection without a profile. Merged
  after profile env, so the call site wins on a key clash.
- `-N, --new` — when no explicit name/instance is given, take the first free
  instance slot instead of re-attaching: `ws__tool`, then `ws__tool__2`, `__3`,
  … With an explicit name it is a no-op (the name already makes it distinct).

## Profile schema (global `~/.sloop/config.yaml`)

```yaml
profiles:
  sec:
    tool: claude                        # required: a manifest key (or alias)
    env:
      CLAUDE_CONFIG_DIR: ~/.claude-sec   # ~ and $VAR expanded at launch
```

Profiles are optional and unknown to older configs: a config without a
`profiles:` key is unaffected.

## Session naming

A named instance appends a suffix: `ws__tool__instance`. An empty instance is
the legacy `ws__tool`, byte-for-byte unchanged. The profile name (or `--name`,
or the auto-picked `2`, `3`, …) becomes the instance suffix, and the session
registry records it.

## Tool-aware name parsing (the one careful bit)

The fleet view splits a session name back into workspace and tool. A
three-segment `ws__tool__instance` breaks the old "split on the last `__`" rule
— it would read the instance as the tool. Instead the tool is identified **by
matching known adapter keys**: the tool is the last `__` segment that is a known
tool (legacy `ws__tool` → no instance), else the segment just before a trailing
instance. This is backward compatible, and robust even when a workspace name
itself contains `__`, because the tool is found by key rather than by counting
separators.

## Env injection

Launch carries extra env (e.g. `CLAUDE_CONFIG_DIR`). Under tmux the launched
command is prefixed with the `env` binary, so it works on every tmux version
with no dependency on `new-session -e`:

```
new-session -d -s <session> -c <dir> env CLAUDE_CONFIG_DIR=… <launch> <args…>
```

Without tmux, the env is set on the child process directly. Values are expanded
before launch: a leading `~/` becomes the home dir, and `$VAR`/`${VAR}` expand
against the current environment. Empty env changes nothing (identical to today).

## Fleet display

The instance shows next to the tool so two claudes in one repo are
distinguishable — `claude·sec`, `claude·b`, `claude·2` — consistently across
`sloop ps`, the fleet picker, the per-session status line, and the waiting
badge.

## `sloop profile` command

A small group that edits the global config (hand-editing the YAML stays valid):

```
sloop profile add <name> --tool <tool> [--env KEY=VAL …]   # write/overwrite
sloop profile ls                                            # name · tool · env keys
sloop profile rm <name>                                     # delete
```

`add` validates that the tool resolves to a known adapter before saving; `ls`
with no profiles prints a hint pointing back at `profile add`.

## Error handling

User-facing messages stay specific and actionable:

- Unknown profile → points at `sloop profile ls`.
- A profile whose `tool` is not a known adapter → names the bad tool.
- An instance name containing `__` → rejected (reserved separator).
- An `--env` value that is not `KEY=VAL` → rejected.
- `profile rm` of a name that does not exist → says so.
