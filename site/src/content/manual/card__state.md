---
slug: "card/state"
command: "hearthroom card state"
short: "Show or manage the shared state players write on the linked card"
parent: "card"
---

# hearthroom card state

Shared state is what rules.json declares under sharedState: polls players vote
in and counters they add to (co-op goals, leaderboards). Players can write only
after they have played the card; the platform stores numbers and declared option
ids, never player text.

  state show [dir]            live and draft totals, the switches and the declaration
  state off|on [dir]          stop or resume player reads and writes (data is kept)
  state reset <key> [dir]     set one key back to zero (--draft for your playtest data)
  state clear-draft [dir]     drop everything written while you playtested the draft

Your own playtests (Play draft on the card page, private cards) write to the draft data and
never mix into what players see.

## Commands

- [`hearthroom card state clear-draft`](/manual/card/state/clear-draft/) — Drop the data written while you playtested the draft
- [`hearthroom card state off`](/manual/card/state/off/) — Stop player reads and writes (data is kept)
- [`hearthroom card state on`](/manual/card/state/on/) — Resume player reads and writes
- [`hearthroom card state reset`](/manual/card/state/reset/) — Set one key back to zero
- [`hearthroom card state show`](/manual/card/state/show/) — Show live and draft totals and the switches

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, check, push, validate, render, preview, pull, import, list, rate, submit
