# Config — the three layers

Sloop keeps a clear split: **user config is unified** (one local + one global file), and only the
*provider knowledge* (adapter manifests, see `docs/ADAPTERS.md`) is per-provider. This matches how
peers do it — aider has a single `.aider.conf.yml`; Claude Squad a single `~/.claude-squad/config.json`
with programs listed inside — while sloop's extra global/local split fits its cross-repo + per-project
nature.

## 1. Global — `~/.sloop/` (machine-wide)

| Path | Purpose |
|---|---|
| `config.yaml` | machine prefs: `version`, `mode` (`ask`/`auto`) |
| `sloop.db` | workspace registry + session history (SQLite, pure-Go) |
| `state/` | hook-written status markers (`<session>.json`) read by `sloop ps` |
| `adapters/` | user adapter overrides / new tools (`<tool>.yaml`) |

```yaml
# ~/.sloop/config.yaml
version: 1
mode: ask
lang: vi        # optional: hint language (en/vi); empty = auto from $SLOOP_LANG/$LANG
hints: false    # optional: turn the 💡 education tips off (omit/true = on)
```

## 2. Local — `.sloop/` (per repo, committed)

| Path | Purpose |
|---|---|
| `config.yaml` | project config: `version`, `tools`, `default_tool`, `mode?` |
| `skills/` | reusable `*.md` skills (symlinked into each tool) |
| `vault/` | personal notes (not delivered to tools) |
| `.gitignore` | ignores local caches |

```yaml
# .sloop/config.yaml
version: 1
tools: [claude, codex, cursor, gemini]   # which tools are enabled (the single source)
default_tool: claude
```

Plus, at the repo root: `AGENTS.md` (canonical context, hand-authored, committed) and pointer files
(`CLAUDE.md`/`GEMINI.md`, create-if-missing).

## 3. Built-in — adapter manifests

Provider knowledge (`detect/launch/context/skills/hooks`), embedded in the binary, overridable in
`~/.sloop/adapters/`. See `docs/ADAPTERS.md`. **Not** user config.

## `version` and forward-compat

Both config files carry `version: 1`. Configs without it still load (defaulted). When the schema
changes, bump `version` and migrate on load.

### Roadmap homes (why config stays simple)

Each planned direction has its own home, so user config doesn't fragment per provider:

| Direction | Lives in |
|---|---|
| Registry (skills / adapters) | `.sloop/skills.lock`, `~/.sloop/adapters/` |
| AI-awareness (status/hooks) | adapter manifest `hooks:` + `~/.sloop/state/` |
| DX / AI-usage | CLI flags + shell completion |
| Guidelines | `AGENTS.md` + `.sloop/skills/` |
| 2nd brain | `.sloop/vault/` + `sloop.db` |

There are **no per-tool user config files**. If named launch configs are ever wanted (e.g.
`reviewer = claude + args`), they return as a `profiles:` **map inside** `.sloop/config.yaml`
(gated by `version`), not as separate files.
