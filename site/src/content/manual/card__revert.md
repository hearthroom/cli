---
slug: "card/revert"
command: "hearthroom card revert"
short: "Revert a card's draft to its published version"
parent: "card"
---

# hearthroom card revert

Replace this card's saved draft with its published version. Preview by default; add --confirm to save the change. This command has no undo. The published version stays unchanged. Linked library Lorebooks use their current content.

Local card folders are unchanged. Pull the saved draft afterwards if you keep a local copy.

## Usage

```
hearthroom card revert <roleId> [flags]
```

## Options

```
      --confirm   Replace and save the draft; this command has no undo
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
