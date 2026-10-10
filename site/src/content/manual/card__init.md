---
slug: "card/init"
command: "hearthroom card init"
short: "Create a new card folder"
parent: "card"
---

# hearthroom card init

Create a new card folder

## Usage

```
hearthroom card init <dir> [flags]
```

## Options

```
      --language string   the language the card is written in (required): zh-Hant, zh-Hans, en, ja, ko
      --name string       card name (default: folder name)
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
