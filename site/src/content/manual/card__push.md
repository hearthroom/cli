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
frees the oldest). The website's card inventory does not list trial cards.
To keep the card, push with --create once. The new card appears in your
inventory and defaults to unlisted: anyone with its link can play the latest
saved content, including changes that have not passed review.
The folder remembers the new card and later pushes update it.
--to <roleId> writes into a card you already own.

To change an owned card's visibility, use card visibility set private <dir>,
set unlisted <dir> or set public <dir>. Creating a card does not submit it for
review or add it to public listings.

The Play link it prints opens your saved draft (?mode=source). The website's
"Play draft" button opens the same draft.

For public cards, players keep the approved version until an update passes
review. For unlisted cards, anyone with the link can play the latest saved
content, including changes that have not passed review. A push preserves the
card's current visibility and does not submit it for review.

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
provider's entry limit; use --create for a larger owned card. New owned cards
default to unlisted; anyone with the link can play the latest saved content.

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
      --create            create a new owned card (unlisted by default) and write to it
      --dry-run           show what would be sent without sending
      --evict             free the least recently used trial slot when all are in use
      --force             send every section even if unchanged; also skips the readiness check
      --prune             delete CLI-managed Lorebook entries removed from the local file
      --skip-media        do not upload assets; keep previously uploaded URLs
      --to string         write to a card you own by id instead of a trial card
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
