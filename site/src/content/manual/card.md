---
slug: "card"
command: "hearthroom card"
short: "Work with card folders: init, push, validate, pull, import, list"
parent: "hearthroom"
---

# hearthroom card

Work with card folders: init, push, validate, pull, import, list

## Commands

- [`hearthroom card import`](/manual/card/import/) — Convert a SillyTavern PNG/JSON/CHARX card or an MMD file set into a card folder
- [`hearthroom card init`](/manual/card/init/) — Create a new card folder
- [`hearthroom card list`](/manual/card/list/) — List your cards on the provider
- [`hearthroom card pull`](/manual/card/pull/) — Write one of your cards to a folder
- [`hearthroom card push`](/manual/card/push/) — Sync the folder to the provider (a private trial card by default)
- [`hearthroom card status`](/manual/card/status/) — Show what a folder is linked to and which sections changed
- [`hearthroom card validate`](/manual/card/validate/) — Show the provider's pre-publish validation for the pushed card
- [`hearthroom card view`](/manual/card/view/) — Show a community card

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://hearthroom.club)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
