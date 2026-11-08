---
title: Trial cards and real cards
---

# Trial cards and real cards

`hearthroom card push` has two kinds of target. Knowing which one you are writing to answers most "where did my card go?" questions.

## Trial card (the default)

```sh
hearthroom card push my-card
```

- A private card on the provider, made for trying what you just edited.
- Expires three days after the last push; each push renews it.
- Each account keeps at most five; `--evict` frees the oldest when they are all in use.
- Playable at once through the link the CLI prints, and with `hearthroom play`.
- Not listed in the website's card inventory: it is a sandbox, not a work.

The folder remembers its trial card in `.hearthroom/state.json`, so later pushes update the same one.

## Real card

```sh
hearthroom card push my-card --create
```

- Creates a card that stays and appears in your inventory, unlisted by default. Anyone with its link can play the latest saved content, including changes that have not passed review; it stays out of public listings.
- After `--create` the folder is linked to that card; plain `hearthroom card push my-card` keeps updating it.
- `--to <roleId>` links the folder to a card you already own instead, for example one you pulled with `hearthroom card pull`.

## One source folder, several cards

A folder remembers one default target in `.hearthroom/state.json`. After
`push --create`, `push --to <id>` or pulling an owned card, a plain `push`
updates that owned card. It does not switch back to a trial card. Use
`card status <dir>` to check the current link.

You can use one complete source folder for a private backup and another owned
card. Set `BACKUP_ID` and `FORMAL_ID` to your own card IDs, then push the same
files to each:

```sh
hearthroom card status my-card
hearthroom card push my-card --to "$BACKUP_ID" --force
hearthroom card push my-card --to "$FORMAL_ID" --force
```

Switching from one linked owned card to another requires `--force`; the CLI
sends the folder's current content to the new target. If edit A remains in the
folder when you add edit B, both go to the target, even if that target never
received A before. There is no merge between remote cards. Each push preserves
the target's visibility and does not submit an update for review. Public-card
players still receive the approved copy until an update passes review.

Keep the complete working files in version control. To compare another remote
card, pull it into a new folder, inspect the differences, then apply the wanted
changes to your source. `pull <id> my-card --force` replaces the local source
with that remote copy; pushing it elsewhere can overwrite a newer version.
When optional reads succeed, pull removes obsolete CLI-managed files, including
`lorebook.json`, `rules.json` and extra immediate `openings/*.md` files. It keeps
notes, assets, non-Markdown files and nested directories. Older CLI releases
can leave those optional files behind; a fresh comparison folder avoids mixing
them. Treat pull warnings as an incomplete read and resolve them before pushing.

For trial testing alongside owned cards, keep a separate trial folder with its
own sync state and copy the wanted authoring content into it. There is no
`--trial` switch. Pulling a trial card does not recover its original trial key,
so the next plain push from that pulled folder creates a new trial slot.

## Which one when

| Situation | Command |
|---|---|
| Iterating on the text, playing a turn between edits | `card push` |
| Ready to keep it, share it, or submit it | `card push --create` once, then `card push` |
| Editing a card that already exists on the site | `card pull <id> dir`, edit, `card push dir` |
| Checking what would be sent | `card push --dry-run` |

## What a push does

1. Uploads any asset the folder references and rewrites those references to their served URLs in the payload (local files keep relative paths).
2. Writes the card in sections (fields, opening, Lorebook, display rules) and reports which sections changed.
3. With `--validate`, fetches the provider's pre-publish report and shows its character limits next to your counts.

The CLI never deletes remote content on its own. Removing a Lorebook entry locally removes it remotely only if the CLI created or pulled that entry.

## Large Lorebooks and interrupted pushes

The CLI reads the provider's limits and splits Lorebook writes into batches (the
current provider allows 200 operations per request). Trial cards hold at most
1000 entries; the CLI checks the expanded entry count before any upload and suggests
`--create` when the card is too large for a trial slot.

After an interrupted push, rerun the same command with the same files. The saved
`.hearthroom/state.json` reuses the card and Lorebook and continues from its progress.
Keep that file. If the last reply was lost, the CLI reads the entries back before
repeating creates; an ambiguous result stops with a message to check the remote book.

Long entries without group settings are split for upload and listed before any
writes. All parts share `groupId` and have increasing `groupOrder`; joining their
contents exactly restores the original. Your files stay unsplit. Existing author
groups are sent unchanged and remain subject to the provider's content limit.

`--create` defaults to **unlisted**: anyone with the link can play the latest saved content, including changes that have not passed review. Creating a card does not submit it for review or add it to public listings. To change an owned card's visibility, use `hearthroom card visibility set private my-card`, `set unlisted my-card` or `set public my-card`. Later pushes preserve its visibility. Trial cards stay private.
