---
slug: "review/report"
command: "hearthroom review report"
short: "Summarize suggestions by criteria version over a time range"
parent: "review"
---

# hearthroom review report

Summarize suggestions by criteria version over a time range

## Usage

```
hearthroom review report [flags]
```

## Options

```
      --criteria string   criteria version, e.g. v1
      --from int          start time in Unix milliseconds
      --to int            end time in Unix milliseconds
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
