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

- Creates a private card that stays. It appears in the website's card inventory and can be submitted for community review there.
- After `--create` the folder is linked to that card; plain `hearthroom card push my-card` keeps updating it.
- `--to <roleId>` links the folder to a card you already own instead, for example one you pulled with `hearthroom card pull`.

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

`--create` requests **private** access: the author and explicitly authorized clients
can access the card. **Unlisted** access also lets anyone with the link open it.
Creating a card does not publish it or submit it for review.
