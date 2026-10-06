---
slug: "card"
command: "hearthroom card"
short: "Work with card folders: init, check, push, validate, render, preview, pull, import, list"
parent: "hearthroom"
---

# hearthroom card

Work with card folders. A card is a folder of plain files (see
https://cli.hearthroom.club/guides/card-folder/). init and import write an
AGENTS.md into the folder so a coding agent knows the loop and where the
card-writing skills are: https://github.com/hearthroom/skills.

## Commands

- [`hearthroom card check`](/manual/card/check/) — Local checks of the folder before a push: rules, markers, sandbox API use, protocol health
- [`hearthroom card import`](/manual/card/import/) — Convert a SillyTavern PNG/JSON/CHARX card or an MMD file set into a card folder
- [`hearthroom card init`](/manual/card/init/) — Create a new card folder
- [`hearthroom card list`](/manual/card/list/) — List your cards on the provider
- [`hearthroom card preview`](/manual/card/preview/) — Open the folder in the real sandbox chat shell with a fake host, offline
- [`hearthroom card pull`](/manual/card/pull/) — Write one of your cards to a folder
- [`hearthroom card push`](/manual/card/push/) — Sync the folder to the provider (a private trial card by default)
- [`hearthroom card render`](/manual/card/render/) — Show an opening after the card's display rules, as the player's renderer receives it
- [`hearthroom card status`](/manual/card/status/) — Show what a folder is linked to and which sections changed
- [`hearthroom card validate`](/manual/card/validate/) — Show the provider's pre-publish validation for the pushed card
- [`hearthroom card view`](/manual/card/view/) — Show a community card

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
