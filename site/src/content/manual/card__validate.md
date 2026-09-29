---
slug: "card/validate"
command: "hearthroom card validate"
short: "Show the provider's pre-publish validation for the pushed card"
parent: "card"
---

# hearthroom card validate

Show the provider's pre-publish validation for the pushed card

## Usage

```
hearthroom card validate [dir] [flags]
```

## Options

```
      --push     push the folder first so the report reflects the current files
      --strict   exit non-zero on warnings as well as blockers
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, push, validate, pull, import, list
