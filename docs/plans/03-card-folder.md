# Plan 03 — Card folder format, `card init`, `card pull`

## Goal

Define the on-disk shape of a card so that authors and agents edit plain files,
and the CLI can sync them section by section. The format is the one public
contract this project invents; it is versioned and documented in
`docs/card-folder.md`.

## Layout (`formatVersion: 1`)

```
my-card/
  card.json          manifest and short structured fields
  definition.md      private definition (roleDetailDesc)
  welcome.md         opening message (roleWelcome)
  openings/          optional alternate openings, one Markdown file each
    alt-01.md
  lorebook.json      optional Lorebook: name, format, entries[]
  rules.json         optional display rules (author asset): rules[], mountTrigger, mountLayer, pageMode
  assets/            media referenced by relative path from card.json or text
  .hearthroom/
    state.json       sync state: roleId, trial key, section hashes, uploaded asset digests
```

`card.json`:

```json
{
  "formatVersion": 1,
  "name": "…",
  "summary": "…",              // roleDesc
  "tags": ["…"],               // roleTag
  "type": "story",             // roleType: companion | story | game | generator
  "sex": "",                   // roleSex (stored as-is)
  "playerName": "",            // userName
  "nickname": "",
  "outputContract": "",        // roleOutputContract
  "customInstructions": "",
  "talkExample": [],
  "prologue": [],              // player-side first lines
  "cardMeta": {},
  "media": {
    "portrait": "assets/portrait.png",   // roleAvatar / roleBackground (same asset on HarperHarbor)
    "background": ""
  }
}
```

Mapping to the four trial-card sections:

| Section | Files |
|---|---|
| `card` | `card.json` (minus `prologue`, `media` resolved to URLs) + `definition.md` |
| `welcome` | `welcome.md`, `openings/*.md`, `card.json.prologue` |
| `worldbook` | `lorebook.json` |
| `authorAsset` | `rules.json` |

Long text lives in Markdown files because agents edit those well; short
structured fields live in one JSON manifest. Unknown keys in `card.json` are
preserved on round-trip and reported, never dropped silently.

## `card init [dir]`

Creates the folder with a minimal `card.json`, empty `definition.md` and
`welcome.md`, and a `.hearthroom/` directory. Refuses to overwrite.

## `card pull <roleId> [dir]`

Reads an owned card with `GET /open/v1/role/detail`, its Lorebook bindings and
entries (`/open/v1/worldbook/bindings`, `/open/v1/worldbook/entry/list`), and
its display rules (`/open/v1/role/author-asset`), then writes the folder.
Referenced media URLs are kept as URLs in `card.json.media`; `--download`
fetches them into `assets/` and rewrites the reference.

`pull` into an existing folder refuses unless `--force`, and always shows
which files would change first (`--dry-run`).

## Tests

- Round-trip: folder → payload → folder produces identical files.
- Section hashes are stable across runs and change only when their files change.
- Unknown `card.json` keys survive a load/save cycle.

## Acceptance

- `card init`, edit, `card push` (plan 04), `card pull` back into a new folder
  yields the same content.
