---
slug: "card/import"
command: "hearthroom card import"
short: "Convert a SillyTavern PNG/JSON/CHARX card or an MMD file set into a card folder"
parent: "card"
---

# hearthroom card import

Reads cards from other tools and writes a card folder. Nothing is sent to the
provider; review the folder, then run "hearthroom card push".

Formats are detected by content:
  - SillyTavern PNG (V2 "chara" or V3 "ccv3" text chunk), JSON (V1/V2/V3), CHARX (zip)
  - MMD three-file set: regex export JSON, World Info JSON and persona TXT, in any order;
    pass all files of the set in one call (a PNG among them is treated as a SillyTavern card)

Every source field that has no place in the folder is listed in the report.

## Usage

```
hearthroom card import <file>... [flags]
```

## Options

```
      --force             overwrite an existing card folder
      --language string   card language: picks the creator note and sets the Lorebook entry length limit (en, zh-Hant, zh-Hans, ja, ko) (default "en")
      --mmd               treat JSON inputs as MMD set parts even if they look like a card
      --out string        folder to write (default: derived from the card name)
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, check, push, validate, render, preview, pull, import, list
