# Card folder format (formatVersion 1)

A card is a folder. Long text lives in Markdown files so people and agents can
edit it comfortably; short structured fields live in one JSON manifest. The
CLI syncs the folder to the provider section by section and keeps its sync
state in a hidden directory.

```
my-card/
  card.json          manifest (required)
  definition.md      the private definition of the character (roleDetailDesc)
  welcome.md         the opening message (roleWelcome)
  openings/          optional alternate openings, one Markdown file each, in file-name order
    alt-01.md
  lorebook.json      optional Lorebook
  rules.json         optional display rules ("author asset")
  assets/            media referenced by relative path
  AGENTS.md          written by init and import for coding agents; never sent to the provider
  README.md          optional notes; never sent to the provider
  .hearthroom/
    state.json       sync state written by the CLI
```

## card.json

```json
{
  "formatVersion": 1,
  "name": "Mira",
  "summary": "A lighthouse keeper on a stormy island.",
  "tags": ["drama", "slow-burn"],
  "type": "story",
  "sex": "",
  "playerName": "",
  "nickname": "",
  "language": "en",
  "outputContract": "",
  "customInstructions": "",
  "talkExample": [
    {"roleType": "user", "content": "Is the light on tonight?"},
    {"roleType": "ai", "content": "It always is."}
  ],
  "prologue": ["You arrive at dusk."],
  "cardMeta": {"creator": "…", "characterVersion": "1.0"},
  "media": {
    "portrait": "assets/mira.png",
    "background": ""
  }
}
```

| Key | Provider field | Notes |
|---|---|---|
| `name` | `roleName` | required |
| `summary` | `roleDesc` | short public intro |
| `tags` | `roleTag` | |
| `type` | `roleType` | `companion`, `story`, `game`, `generator` |
| `sex` | `roleSex` | stored as-is |
| `playerName` | `userName` | what the card calls the player |
| `nickname` | `nickname` | what `{{char}}` expands to when it differs from `name` |
| `language` | card language | informational on push; set on create |
| `outputContract` | `roleOutputContract` | |
| `customInstructions` | `customInstructions` | |
| `talkExample` | `talkExample` | `roleType` is `user` or `ai` |
| `prologue` | opening prologue options | player-side first lines |
| `cardMeta` | `cardMeta` | Character Card V3 provenance, kept verbatim |
| `media.portrait` | `roleAvatar` | relative path under `assets/` or an absolute URL |
| `media.background` | `roleBackground` | same; on HarperHarbor the portrait doubles as the background |

Unknown keys are preserved on load and save and never sent to the provider.

## Markdown files

- `definition.md` → `roleDetailDesc`. `welcome.md` → `roleWelcome`.
- `openings/*.md` → alternate openings, sorted by file name.
- A file whose only content is an HTML comment (`<!-- … -->`) counts as empty.
- Text is trimmed. Line endings are normalised to `\n`.
- Any `assets/…` reference inside these files, Lorebook entries or rule
  replacements is uploaded on push and rewritten to its served URL in the
  payload. The local files keep the relative path.

## lorebook.json

```json
{
  "name": "Island",
  "entries": [
    {
      "name": "The lamp",
      "content": "The lamp burns whale oil and must be trimmed at dusk.",
      "keywords": ["lamp", "light"],
      "secondaryKeywords": [],
      "constant": false,
      "disabled": false,
      "triggerRegion": "both",
      "matchOptions": null
    }
  ]
}
```

`id` fields appear after `pull` or a push to an owned card; they hold the
provider's ids and let later pushes update instead of duplicate. `disabled`
entries are not sent to trial cards.

## rules.json

```json
{
  "rules": [
    {"id": "bold", "name": "bold", "find": "\\*\\*(.+?)\\*\\*", "replace": "<b>$1</b>", "enabled": true}
  ],
  "mountTrigger": "",
  "mountLayer": "under",
  "pageMode": "classic"
}
```

`mountLayer` is `under`, `over` or `cover`; `pageMode` is `classic`,
`immersive` or `sandbox`. `id` defaults to `rule-NN`.

## Sections and sync

The provider stores a trial card in four sections. The CLI hashes the payload
it would send for each section and compares it with the hashes it stored after
the last push, so only changed sections are sent.

| Section | Built from |
|---|---|
| `card` | `card.json` (except `prologue` and `media`, which are resolved) + `definition.md` |
| `welcome` | `welcome.md`, `openings/*.md`, `card.json.prologue` |
| `worldbook` | `lorebook.json` |
| `authorAsset` | `rules.json` |

## .hearthroom/state.json

Written by the CLI; safe to delete (the next push re-uploads everything and
creates a new trial card). Contains the target (`trial` or `owned`), the
provider role id, the trial key, the API base, section hashes, uploaded asset
digests and URLs, the Lorebook id and the author-asset version. Commit it if
you want teammates to push to the same card; ignore it if not.
