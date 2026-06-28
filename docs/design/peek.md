# Design: `sloop peek` — overlay into a waiting agent without losing your spot

**Date:** 2026-06-28
**Status:** Approved design, ready for implementation plan
**Scope:** Phase 1 + Phase 2 (Phase 3 deferred)

## Problem

When you orchestrate a cross-repo fleet, you work from one "home base" session
while agents run in other tmux sessions. When an agent hits a hook that needs
your input (it goes `waiting`), the only way to answer it today is
`sloop attach`, which — because `attach` cannot nest inside tmux — falls back to
`switch-client` (`internal/tmux/fleet.go:27`). That **replaces your whole
screen** with the agent's session. You answer, then `switch-client` back. You
lose your spot and your context every round trip.

What we actually want: from home base, get a quiet signal that an agent needs
you, press one key to **float that agent's live pane over your current screen**,
answer it in place, dismiss it, and be exactly where you were.

This is the "peek and drop" flow. It is the unique thing tmux overlays enable
that a screen-replacing `switch-client` cannot.

## Non-goals (deliberately deferred)

- **Phase 3 — non-modal floating panes (tmux ≥ 3.7 `new-pane`).** A modal
  `display-popup` is well-matched to "answer and dismiss," so non-modal is
  polish, not foundation. It becomes a progressive enhancement behind a future
  `FloatingSupported()` (≥ 3.7) once 3.7 is widespread. Not in this spec.
- **True auto-pop.** We will not auto-open an overlay onto the user mid-keystroke.
  Surfacing is always user-triggered. A future opt-in config knob can add
  aggressive auto-pop for those who want it.
- **Watching multiple agents floating at once**, move/resize, custom layouts.

## Design overview: "Signal → Act"

Two layers ship here. (The earlier three-layer framing folds the standalone HUD
glance into Phase 3; Phase 1+2 is the convenient core.)

### Layer A — Signal (ambient, never interrupts)

The status hooks already write a fresh `waiting | working | idle` marker per
session (`internal/fleetstate/state.go`, read via `fleetstate.Read`). We surface
a **fleet-wide waiting count** on the home-base session's status line, e.g.
`⏳ 2 waiting`, rendered empty when zero so the bar stays clean.

The status line is drawn on every window/client the user looks at, so this
signal "follows the user" for free, on any tmux version, with zero interruption.

- New hidden render command (called by tmux `#()`), parallel to the existing
  per-session `statusline` (`internal/cli/commands/statusline.go`). Proposed:
  `sloop statusline fleet` → counts `waiting` across all sloop sessions
  (`tmux.ParseSessions(tmuxList())` + `fleetstate.Read`, with the same
  capture-pane fallback `renderStatusline` already uses) and prints a short
  badge to stdout (empty string when zero).
- A setup path adds it to `status-right` of the home-base session, mirroring the
  existing `statusline setup` that sets `status-left`-style per-session output.

### Layer B — Act (`sloop peek`)

A new command that overlays a target agent's **live session** in a
`display-popup`, using a nested attach so the home-base client is never touched.

```
sloop peek [agent]
  no arg  → resolve target (see below)
  agent   → peek that session directly
```

**Target resolution (the "convenient" part):**
1. exactly one agent is `waiting` → peek it directly, no prompt.
2. more than one `waiting` → show the fleet picker (`pickFleetSession`, which is
   already sorted waiting-first) so you choose.
3. none `waiting` → fall back to the same picker over the whole fleet.

**The overlay mechanism (the one tricky part):**
`display-popup -E` runs its command with `$TMUX` still set, so a plain
`tmux attach` inside it would be refused ("sessions should be nested with care").
We clear `TMUX` for the inner command so tmux allows a **fresh nested client**:

```
display-popup -w 90% -h 80% -E 'TMUX= <mux> attach -t <session>'
```

Closing the popup kills that nested client → it detaches → **the agent session
keeps running**. The home-base client was never switched.

Caveats to verify during implementation (call-outs for the plan, not blockers):
- Nested attach clamps the session's window size to the smallest attached
  client while the popup is open. Acceptable for a peek; note it.
- Confirm closing the popup detaches cleanly and never kills the agent session.
- Confirm `TMUX=` (and clearing `TMUX_PANE` if needed) reliably permits the
  nested attach across tmux 3.2–3.6.

**Keybind without double-popup:**
Pressing the bound key must open exactly one popup. So the keybind points
straight at the popup, and the *inner* command does target resolution (it has a
TTY inside the popup) then `exec`s the nested attach:

```
bind-key p display-popup -w 90% -h 80% -E "<sloop> peek --in-popup"
```

- `sloop peek` (from a normal shell) wraps the inner command in the popup itself.
- `sloop peek --in-popup` (flag name TBD in plan): already inside a popup —
  resolve target, then exec the `TMUX=`-cleared nested attach. No extra popup.
- `sloop peek setup [--key p]` installs the bind on the live server and prints
  the `~/.tmux.conf` line, mirroring `popup setup`
  (`internal/cli/commands/popup.go`).

## Component breakdown

| Unit | Location (proposed) | Responsibility |
|---|---|---|
| `NestedAttachCmd(session) string` | `internal/tmux/` | Build the `TMUX=`-cleared nested-attach shell command (testable seam, like `attachArgs`). |
| `BuildPeekPopupArgs(w,h,inner)` | `internal/tmux/popup.go` | Reuse `BuildPopupArgs`; peek just supplies a bigger default size. |
| `Peek(session)` | `internal/tmux/popup.go` | Run the outer `display-popup` wrapping the inner command. |
| peek target resolver | `internal/cli/commands/` | 0/1/many-waiting → direct or picker. Reuses `pickFleetSession` + `fleetstate`. |
| `peek` cobra command (+`--in-popup`, `setup`) | `internal/cli/commands/peek.go` | Wire the above; require tmux + `PopupSupported()`. |
| fleet waiting-count render | `internal/cli/commands/statusline.go` | `sloop statusline fleet` badge (empty when zero). |
| fleet statusline setup | `internal/cli/commands/statusline.go` | Add the badge to home-base `status-right`. |

## Error & edge handling

- **Not inside tmux** (`$TMUX` empty): peek has no meaning (nothing to overlay
  over). Error clearly and point to `sloop attach`.
- **tmux < 3.2** (`!PopupSupported()`): no `display-popup`. Error and point to
  `sloop attach`. (Peek's value *is* the overlay; silently switch-clienting
  would defeat the purpose, so we don't.)
- **No sessions / empty fleet:** the resolver surfaces the same "no running AI
  sessions" error `pickFleetSession` already returns.
- **Stale/missing markers:** `fleetstate.Read` already returns `ok=false` for
  stale markers; resolver and badge fall back to the capture-pane heuristic the
  status line already uses, so a missing hook never hides a waiting agent.

## Testing

- **Unit:** `NestedAttachCmd` / `BuildPeekPopupArgs` string shape (seam tests,
  like `TestBuildTmuxAttachArgs`). Resolver target selection across 0/1/many
  `waiting` with injected fleet rows + fleetstate. Fleet badge render: `0 →
  ""`, `N → "⏳ N waiting"`.
- **e2e:** extend `e2e/fleet_test.go` to assert peek's popup arg construction and
  that resolution picks the waiting session; the fully interactive nested attach
  is left to manual verification (the caveats above), since a real attach needs a
  live TTY.

## Why this is the simple, convenient core

- Ships on **today's tmux (≥ 3.2)** — every user, not just 3.7.
- Modal popup fits "answer and dismiss" exactly; no non-modal complexity needed.
- Reuses what exists: status hooks, `fleetstate`, `pickFleetSession`,
  `BuildPopupArgs`, `BuildAttachArgs`, the `popup setup` keybind pattern.
- Keeps orchestration minimal, matching the project's wedge (portable context +
  cross-repo fleet, not heavyweight in-repo orchestration).
- The tmux 3.7 feature that prompted this (floating panes) is correctly placed as
  an optional later enhancement, not a dependency.
