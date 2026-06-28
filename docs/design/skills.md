# Skills — portable, reusable prompts across every tool

A **skill** is a reusable prompt/workflow (a markdown file) that sloop delivers to *every* enabled AI
tool from one source. Author or import once; Claude, Cursor, Codex, … all see the same set. This is
context portability across **tools** *and* **sources** — something single-tool orchestrators don't do.

> Hands-on command reference lives in [USAGE.md §5](../guide/USAGE.md). This doc is the model + the roadmap so
> contributions and integrations start from a clear contract.

## Model

- Skills live in **`.sloop/skills/*.md`** (one workspace, one set).
- They are **symlinked** into each tool's skills dir (e.g. `.claude/skills`), resolved from the
  manifest's `skills.target` ([ADAPTERS.md](../reference/ADAPTERS.md)). A directory symlink means every skill is
  shared at once; it self-heals and falls back to copy when symlinks aren't available.
- `sloop sync` (and `skills new`/`add`) keep the links in place.

Markdown was chosen deliberately: skills are **inert and tool-agnostic**. There is no per-tool
translation layer — the same file is valid everywhere. (Contrast with hooks, which execute and are
tool-specific; see [hooks.md](hooks.md).)

## Sources & the lockfile

Skills come from two places:

- **Authored** — `sloop skills new <name>` scaffolds a local skill. No upstream; stays out of the lock.
- **Imported** — `sloop skills add <url|github-blob>` fetches a skill from a source.

Imported skills are recorded in **`.sloop/skills.lock`** (`name` + `source` + `sha256` + fetch time).
Commit the lock so a team gets **reproducible** skills from one source. `sloop skills update [name…]`
re-fetches from the recorded sources and rewrites + relinks only the files whose content changed.

```yaml
# .sloop/skills.lock
version: 1
skills:
  - name: code-review
    source: https://github.com/owner/repo/blob/main/skills/code-review.md
    sha256: e3b0c44…
    updated: 2026-06-28T10:00:00Z
```

## Roadmap

- **Shipped:** `skills new`, `skills add <url|github>`, `.sloop/skills.lock`, `skills update`.
- **Next — registry:** `skills search` / `skills add <name>` resolving from a curated index (or
  skills.sh). Gated on investigating that index's API/format first. The lockfile is the foundation:
  a registry just resolves a `<name>` to a `source` that lands in the same lock.

## Contributing a skill

1. Write a focused markdown skill (one workflow/prompt, a clear `# Title`).
2. Host it at a stable URL (a GitHub blob URL works — sloop rewrites it to raw).
3. Share the `sloop skills add <url>` line. Teammates' `skills.lock` records it; `skills update`
   keeps everyone in sync.

Open question for the registry phase: naming/namespacing and trust for `add <name>` (a curated index
needs review + provenance, the same concern hooks have — see [hooks.md](hooks.md#trust-model)).
