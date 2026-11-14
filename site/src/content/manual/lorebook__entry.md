---
slug: "lorebook/entry"
command: "hearthroom lorebook entry"
short: "List, read, add, update, or delete individual entries on a linked card's Lorebook"
parent: "lorebook"
---

# hearthroom lorebook entry

Reads and edits individual entries on the remote Lorebook bound to an owned card,
without touching other entries or book-level metadata (name, description, tags).
Use these commands when an author and an agent—or multiple sessions—edit the same
Lorebook concurrently.

<entry> accepts an entry id or a unique entry name; when multiple entries share
a name, pass the entry id from "lorebook entry list".

Writes send the Lorebook version from .hearthroom/state.json (or --expected-version)
so concurrent edits return a 409 conflict instead of overwriting each other. After
a write succeeds, the entry and version are updated in local lorebook.json and
.hearthroom/state.json so a later "card push" treats that entry as already synced.
Markdown source files under worldbook/*.md are not rewritten automatically.

## Commands

- [`hearthroom lorebook entry add`](/manual/lorebook/entry/add/) — Add one entry to the linked card's remote Lorebook
- [`hearthroom lorebook entry delete`](/manual/lorebook/entry/delete/) — Delete one entry from the linked card's remote Lorebook
- [`hearthroom lorebook entry get`](/manual/lorebook/entry/get/) — Read one entry from the linked card's remote Lorebook
- [`hearthroom lorebook entry list`](/manual/lorebook/entry/list/) — List entries in the linked card's remote Lorebook
- [`hearthroom lorebook entry update`](/manual/lorebook/entry/update/) — Update one entry in the linked card's remote Lorebook

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom lorebook`](/manual/lorebook/) — Build or check lorebook.json, or read and edit individual Lorebook entries
