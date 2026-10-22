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

The Play link it prints opens your saved draft (?mode=source). Once a card has
passed review, players keep the approved copy until you submit the update for
review; the website shows the owner a "Play draft" button and an "edited since
review" notice until then.

Files under assets/ go to one folder of your media library, named after the
card unless media.folder in card.json names it: assets/art/a.webp is served at
<libraryPrefix>/<folder>/art/a.webp. A referenced directory ("assets/art/", or
"assets/art/$1.webp" in a display rule) uploads every file in it and becomes
that folder's URL, so card code can add file names at runtime. Changing
media.folder uploads the files again under the new name; the old copies stay
in the library until you remove them.

Files are uploaded several at a time (--concurrency, default 6, or
HEARTHROOM_UPLOAD_CONCURRENCY), and the folder state is saved while they go,
so a push that is interrupted picks up with the files that are left.

Lorebook writes use the provider's batch limits. Progress is saved in
.hearthroom/state.json after the card, Lorebook and each batch are saved;
rerun the same command after an interruption to continue. Keep that state file.
Long entries without group settings are split at paragraph or sentence boundaries,
with shared groupId and increasing groupOrder; the original text stays in your
files. Existing group settings are sent as written. Trial cards hold at most the
provider's entry limit; use --create for a larger private draft. Private cards
are accessible to the author and explicitly authorized clients; unlisted cards
can also be opened by anyone with their link.

Disabled Lorebook entries are uploaded and remain disabled. Entries removed
locally are deleted from an owned card only with --prune; otherwise push lists
them and stops before writing the Lorebook. Entries created by other tools are kept.

## Usage

```
hearthroom card push [dir] [flags]
```

## Options

```
      --concurrency int   files uploaded at once, 1 to 16; HEARTHROOM_UPLOAD_CONCURRENCY changes the default (default 6)
      --create            create a new private card and write to it
      --dry-run           show what would be sent without sending
      --evict             free the least recently used trial slot when all are in use
      --force             send every section even if unchanged; also skips the readiness check
      --prune             delete CLI-managed Lorebook entries removed from the local file
      --skip-media        do not upload assets; keep previously uploaded URLs
      --to string         write to an owned private card by id instead of a trial card
      --validate          run the server validation after pushing
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
