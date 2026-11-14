---
slug: "lorebook/entry/update"
command: "hearthroom lorebook entry update"
short: "Update one entry in the linked card's remote Lorebook"
parent: "lorebook/entry"
---

# hearthroom lorebook entry update

Update one entry in the linked card's remote Lorebook

## Usage

```
hearthroom lorebook entry update [dir] <entry> [flags]
```

## Options

```
      --category string              entry category: rule, character, location, item, event, or custom
      --constant                     always include the entry without keyword matching
      --content string               entry body text
      --content-file string          read entry body text from a plain file ('-' for stdin)
      --disabled                     keep the entry saved but inactive
      --expected-version int32       require this remote Lorebook version before writing
      --file string                  read entry from a Markdown source file or JSON object ('-' for stdin)
      --force                        write without checking the remote Lorebook version
      --keywords strings             primary trigger keywords (comma-separated or repeatable)
      --match-options string         JSON object for advanced matchOptions
      --name string                  entry title (up to 120 characters)
      --secondary-keywords strings   secondary filter keywords (comma-separated or repeatable)
      --semantic-recall              allow semantic recall in addition to keyword matching (default true)
      --trigger-region string        messages scanned for keywords: both, user_only, or ai_only
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom lorebook entry`](/manual/lorebook/entry/) — List, read, add, update, or delete individual entries on a linked card's Lorebook
