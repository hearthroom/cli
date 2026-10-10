---
slug: "card/list"
command: "hearthroom card list"
short: "List your cards on the provider"
parent: "card"
---

# hearthroom card list

List your cards on the provider

## Usage

```
hearthroom card list [flags]
```

## Options

```
      --limit int   entries per page (default 50)
      --page int    page number (default 1)
      --q string    filter by name
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, check, push, validate, render, preview, pull, import, list, rate, submit
