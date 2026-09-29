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
