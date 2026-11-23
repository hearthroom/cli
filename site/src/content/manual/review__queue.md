---
slug: "review/queue"
command: "hearthroom review queue"
short: "List review submissions"
parent: "review"
---

# hearthroom review queue

Lists review submissions. --scope pending (default) lists submissions waiting
for a decision; recent lists those submitted in the last 7 days, decided or not.
--unshadowed keeps only submissions you have not given a verdict for yet (for
the --criteria version, when given). --count prints {"count": n} instead of items.

## Usage

```
hearthroom review queue [flags]
```

## Options

```
      --count             print only the number of matching submissions
      --criteria string   criteria version for --unshadowed, e.g. v1
      --limit int         maximum items (1-200) (default 50)
      --scope string      pending or recent (default "pending")
      --unshadowed        only submissions you have not given a verdict for
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom review`](/manual/review/) — Staff tools for card review (the server decides who may use them)
