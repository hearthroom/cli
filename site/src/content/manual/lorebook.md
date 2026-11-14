---
slug: "lorebook"
command: "hearthroom lorebook"
short: "Build or check lorebook.json, or read and edit individual Lorebook entries"
parent: "hearthroom"
---

# hearthroom lorebook

A Lorebook is easier to write and review as one file per entry. Keep the sources
under <dir>/worldbook/, one Markdown file each: a frontmatter block (name, keywords,
secondaryKeywords, constant, disabled, order, scanDepth, matchWholeWords,
caseSensitive, selective, selectiveLogic; an optional stable id) followed by the
content. One file holds one entry: a second frontmatter block is an error. Subfolders
are read; files and folders starting with "_" are notes and are skipped. "order" sets
the entry's position in lorebook.json and is not stored as a field. "build" writes
lorebook.json from them (never the other way round); "check" compares the two and
looks for keywords that fire several entries at once, entries nothing can reach, and
too many constant entries.

When editing a linked owned card concurrently with the web editor or another agent,
use "lorebook entry" to list, read, add, update, or delete one remote entry at a time.

## Commands

- [`hearthroom lorebook build`](/manual/lorebook/build/) — Write lorebook.json from worldbook/*.md
- [`hearthroom lorebook check`](/manual/lorebook/check/) — Compare worldbook/*.md with lorebook.json and look for keyword problems
- [`hearthroom lorebook entry`](/manual/lorebook/entry/) — List, read, add, update, or delete individual entries on a linked card's Lorebook

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
