# AGENTS.md

## Provider-aware architecture (read before adding provider behavior)

Sloop's reason to exist is **multi-AI-provider** support, so the codebase is provider-aware by
design: **all per-provider knowledge lives in the adapter manifest** (`internal/adapter/builtin/
<tool>.yaml`, user-overridable in `~/.sloop/adapters/`), and every provider-aware feature reads it.

**The rule: never hardcode a tool name in a feature. Add capability to the manifest instead.**
Detect, launch, context delivery, skills, hooks, and completion are all manifest-driven. To support a
new CLI, drop in one `<tool>.yaml` — no Go changes unless it needs a brand-new mechanism.

- Provider contract + manifest schema: `docs/reference/ADAPTERS.md`
- Config layering (unified user config vs per-provider manifests): `docs/reference/CONFIG.md`
- Runtime capability matrix: `sloop tools`

User config is **unified** (one local `.sloop/config.yaml` + one global `~/.sloop/config.yaml` +
the global DB), never split per provider.

## Development workflow

- **Always run `make lint`, `make test`, and `go test -v -tags e2e ./e2e/...` before committing.** This project requires strict adherence to linting rules and 100% passing tests (including E2E tests which require tmux).
