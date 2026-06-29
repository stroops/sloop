# Design: `sloop peek`, overlay into a waiting agent without losing your spot

**Date:** 2026-06-28
**Status:** Shipped
**Scope:** Phase 1 + Phase 2 (Phase 3 deferred)

## Problem

When you orchestrate a cross-repo fleet, you work from one "home base" session
while agents run in other tmux sessions. When an agent hits a hook that needs
your input (it goes `waiting`), the only way to answer it today is
`sloop attach`, which (because `attach` cannot nest inside tmux) falls back to
`switch-client`. That **replaces your whole screen** with the agent's session.
You answer, then switch back. You lose your spot and your context every round
trip.

What we actually want: from home base, get a quiet signal that an agent needs
you, press one key to **float that agent's live pane over your current screen**,
answer it in place, dismiss it, and be exactly where you were.

This is the "peek and drop" flow. It is the unique thing tmux overlays enable
that a screen-replacing `switch-client` cannot.

## Non-goals (deliberately deferred)

- **Phase 3: non-modal floating panes (tmux ≥ 3.7 `new-pane`).** A modal
  `display-popup` is well-matched to "answer and dismiss," so non-modal is
  polish, not foundation. It becomes a progressive enhancement once tmux 3.7 is
  widespread. Not in this scope.
- **True auto-pop.** We never auto-open an overlay onto the user mid-keystroke.
  Surfacing is always user-triggered. A future opt-in config knob could add
  aggressive auto-pop for those who want it.
- **Watching multiple agents floating at once**, move/resize, custom layouts.

## Design overview: "Signal → Act"

Two layers ship here: an ambient signal that an agent needs you, and a one-key
action to answer it.

### Layer A: Signal (ambient, never interrupts)

The status hooks already write a fresh `waiting | working | idle` marker per
session. We surface a **fleet-wide waiting count** on the home-base session's
status line, e.g. `⏳ 2 waiting`, rendered empty when zero so the bar stays
clean. The status line is drawn on every window/client the user looks at, so the
signal "follows the user" for free, on any tmux version, with zero interruption.

Rather than a separate command and a new setup step, the badge is **appended to
the per-session status line that every sloop session already renders**, so it
appears everywhere automatically, with no new setup. It counts sessions whose
fresh marker is `waiting`, **excluding the current session** so it reads "others
need you," using markers only (no pane capture) so it is cheap enough to run
every status interval.

### Layer B: Act (`sloop peek`)

A command that overlays a target agent's **live session** in a `display-popup`,
using a nested attach so the home-base client is never touched.

```
sloop peek [agent]
  no arg  → resolve target (see below)
  agent   → peek that session directly
```

**Target resolution (the "convenient" part):**

1. exactly one agent is `waiting` → peek it directly, no prompt.
2. more than one `waiting` → show the fleet picker (already sorted waiting-first)
   so you choose.
3. none `waiting` → the same picker over the whole fleet.

**The overlay mechanism (the one tricky part):** `display-popup -E` runs its
command with `$TMUX` still set, so a plain `tmux attach` inside it would be
refused ("sessions should be nested with care"). Clearing `TMUX` for the inner
command lets tmux allow a **fresh nested client**:

```
display-popup -w 90% -h 80% -E 'TMUX= <mux> attach -t <session>'
```

Closing the popup ends that nested client → it detaches → **the agent session
keeps running**. The home-base client was never switched. (Confirmed in use:
the nested attach is permitted across tmux 3.2–3.6, and closing the overlay
detaches cleanly without killing the agent. While the popup is open the session
window is clamped to the smallest attached client, acceptable for a peek.)

**Keybind without double-popup:** pressing the bound key must open exactly one
popup. So the keybind points straight at the popup, and the *inner* command
(`sloop peek --in-popup`, which has a TTY inside the popup) does target
resolution and then the `TMUX=`-cleared nested attach, no second popup:

```
bind-key p display-popup -w 90% -h 80% -E "<sloop> peek --in-popup"
```

`sloop peek` from a normal shell wraps the inner command in the popup itself.
`sloop peek setup [--key p]` installs the bind on the live server and prints the
`~/.tmux.conf` line, mirroring `popup setup`.

## Error & edge handling

- **Not inside tmux:** peek has nothing to overlay over. Error clearly and point
  to `sloop attach`.
- **tmux < 3.2:** no `display-popup`. Error and point to `sloop attach`; peek's
  value *is* the overlay, so silently switching the whole screen would defeat the
  purpose.
- **No sessions / empty fleet:** the resolver surfaces the same "no running AI
  sessions" error the fleet picker already returns.
- **Stale/missing markers:** the fleet state already treats stale markers as
  absent, so the resolver and badge fall back to the capture-pane heuristic the
  status line already uses; a missing hook never hides a waiting agent.

## Why this is the simple, convenient core

- Ships on **today's tmux (≥ 3.2)**: every user, not just 3.7.
- A modal popup fits "answer and dismiss" exactly; no non-modal complexity needed.
- Reuses what exists: status hooks, fleet state, the fleet picker, and the
  `popup setup` keybind pattern.
- Keeps orchestration minimal, matching the project's wedge (portable context +
  cross-repo fleet, not heavyweight in-repo orchestration).
- The tmux 3.7 feature that prompted this (floating panes) is correctly placed as
  an optional later enhancement, not a dependency.
