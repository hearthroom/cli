---
slug: "card/push"
command: "hearthroom card push"
short: "Sync the folder to the provider (a private trial card by default)"
parent: "card"
---

# hearthroom card push

Uploads referenced assets, then writes the card in sections.

Without --to or --create the target is a trial card: a private card the
provider keeps for three days after the last push (five per account; --evict
frees the oldest). It is the fastest way to play what you just edited, but the
website's card inventory does not list trial cards. To keep the card, push with
--create once; the folder then remembers the new private card and later pushes
update it. --to <roleId> writes into a card you already own.

## Usage

```
hearthroom card push [dir] [flags]
```

## Options

```
      --create       create a new private card and write to it
      --dry-run      show what would be sent without sending
      --evict        free the least recently used trial slot when all are in use
      --force        send every section even if unchanged; also skips the readiness check
      --skip-media   do not upload assets; keep previously uploaded URLs
      --to string    write to an owned private card by id instead of a trial card
      --validate     run the server validation after pushing
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://hearthroom.club)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, push, validate, pull, import, list
