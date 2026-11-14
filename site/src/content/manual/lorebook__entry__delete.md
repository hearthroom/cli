---
slug: "lorebook/entry/delete"
command: "hearthroom lorebook entry delete"
short: "Delete one entry from the linked card's remote Lorebook"
parent: "lorebook/entry"
---

# hearthroom lorebook entry delete

Delete one entry from the linked card's remote Lorebook

## Usage

```
hearthroom lorebook entry delete [dir] <entry> [flags]
```

## Options

```
      --expected-version int32   require this remote Lorebook version before deleting
      --force                    delete without checking the remote Lorebook version
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
