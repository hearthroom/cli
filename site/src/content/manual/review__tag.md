---
slug: "review/tag"
command: "hearthroom review tag"
short: "Set flag codes on a review submission without changing your verdict"
parent: "review"
---

# hearthroom review tag

Set flag codes on a review submission without changing your verdict

## Usage

```
hearthroom review tag <submissionId> [flags]
```

## Options

```
      --criteria string    criteria version, e.g. v1
      --flag stringArray   flag code (repeatable)
      --model string       model that produced the flags
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
