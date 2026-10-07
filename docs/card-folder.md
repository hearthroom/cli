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
  "responseDefaults": {"perspective": "third_limited", "length": "target", "lengthTarget": "1200"},
  "media": {
    "portrait": "assets/mira.png",
    "background": "",
    "backgroundLandscape": "assets/mira-wide.png"
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
| `language` | card language | set when the card is created; `en` cards get the longer English field limits |
| `outputContract` | `roleOutputContract` | |
| `customInstructions` | `customInstructions` | |
| `talkExample` | `talkExample` | `roleType` is `user` or `ai` |
| `prologue` | opening prologue options | player-side first lines |
| `cardMeta` | `cardMeta` | Character Card V3 provenance, kept verbatim |
| `responseDefaults` | `responseDefaults` | optional author defaults for the player's response preferences (see [Response defaults](#response-defaults)) |
| `media.portrait` | `roleAvatar` | relative path under `assets/` or an absolute URL |
| `media.background` | `roleBackground` | same; on HarperHarbor the portrait doubles as the background |
| `media.folder` | — | media-library folder for `assets/`; defaults to the card name on the first push (see [Media folder](#media-folder)) |
| `media.backgroundLandscape` | `roleBackgroundLandscape` | optional landscape (16:9) background; the chat page prefers it on wide screens and falls back to the portrait one. Both are cropped to cover the screen, so keep important elements in the central 75% |
| `media.share` | `roleShareImage` | optional 1.91:1 link-preview image (1200×630) shown when the card's link is shared on Discord, LINE, X and similar. Put the title and main subject in the centre and keep the sides to background only, since some platforms crop to 2:1 or show a square thumbnail. Without it, previews use `media.backgroundLandscape`, then the portrait |

Unknown keys are preserved on load and save and never sent to the provider.

### Response defaults

Players can tune each conversation under **Response preferences**: who may
write the player character's part, prose style, narrative perspective, reply
length and pacing, each with an optional one-line note. The platform picks a
default for each. `responseDefaults` replaces those defaults for this card,
for example a card that is meant to be read in third person, or one whose
status blocks need long replies:

- A player who has not changed an axis gets your value, and the preferences
  panel shows it selected. "Reset" returns to your value.
- An axis where the player picks a different option uses the player's
  choice, and your note, custom style or length target for that axis stops
  applying with it. Picking your own option again changes nothing: it is
  still your default, with your note and text.
- Changing `responseDefaults` and pushing reaches existing conversations from
  their next reply, except on axes the player has changed.

| Key | Values |
|---|---|
| `agency` | `protect` (never writes the player's part, the platform default), `assist` (fills in the details of actions the player stated), `lines` (may write the player character's lines but not their decisions), `coauthor` (may write the player character's words, actions and decisions) |
| `style` | `default` (platform writing guide, the platform default), `guided` (lighter guide that defers to your material), `card` (no platform guide; your definition and examples set the voice), `custom` (needs `customStyle`) |
| `customStyle` | style description, at most 1000 characters; only with `style: custom` |
| `perspective` | `card` (follow the material, the platform default), `first_character`, `second_user`, `third_limited`, `third_omniscient` |
| `length` | `auto` (the platform default), `recommended` (2 to 5 paragraphs, about 2500 characters), `target` (needs `lengthTarget`) |
| `lengthTarget` | one of 200, 300, 400, 500, 600, 800, 1000, 1200, 1500, 2000, 2500, 3000, 4000, 5000, 7000, 10000 (characters; words for English); only with `length: target`. A string or a number |
| `pace` | `natural` (the platform default), `linger`, `advance` |
| `agencyNote`, `styleNote`, `perspectiveNote`, `lengthNote`, `paceNote` | a one-line refinement of that axis, at most 200 characters; where it differs from the option, the note wins |

Leave out any key to keep the platform default for it. To remove every
default from the server, set `"responseDefaults": {}` and push; deleting the
key from card.json leaves the server's copy untouched. `hearthroom validate`
checks the values before a push.

## Markdown files

- `definition.md` → `roleDetailDesc`. `welcome.md` → `roleWelcome`.
- `openings/*.md` → alternate openings, sorted by file name.
- A file whose only content is an HTML comment (`<!-- … -->`) counts as empty.
- Text is trimmed. Line endings are normalised to `\n`.
- Any `assets/…` reference inside these files, Lorebook entries or rule
  replacements is uploaded on push and rewritten to its served URL in the
  payload. The local files keep the relative path.

## Media folder

Everything under `assets/` is uploaded into one folder of your media library,
and the served URL mirrors the path: `assets/art/expr/happy.webp` becomes
`<libraryPrefix>/<folder>/art/expr/happy.webp`. Paths are case-sensitive;
non-ASCII folder names are percent-escaped in the URL.

- The folder is `media.folder` when set, otherwise the card name. The first
  push records it in `.hearthroom/state.json`, so renaming the card later does
  not move its files. A folder that already held uploads before this field
  existed keeps its old name until `media.folder` is set.
- To rename the card's folder, run `media mv <old> <new>`: it shows which of
  your cards still mention the old URLs, and `--yes` moves the files. A file's
  URL is its path, so the old URLs stop working. Then set `media.folder` to the
  new name. A push also follows files that were renamed or moved on the
  website, and uploads again any that were deleted there.
- Setting `media.folder` to a new name without moving uploads the files again
  under it. The old copies stay in the library; `media ls --q <old>/` and
  `media rm` remove them.
- On the first push into a folder this card has not uploaded to, the CLI stops
  if the folder already holds other files, because an upload with the same
  path replaces that file for every card that uses it. Set `media.folder` to a
  new name, or to that name to share the folder on purpose (one series, one
  folder).
- A directory reference is uploaded whole: `"assets/art/expr/"`, or a path
  with a placeholder such as `assets/art/expr/$1.webp` in a rule replacement or
  `` `assets/art/expr/${mood}.webp` `` in a script. Every file under that
  directory is uploaded and the directory part is rewritten to its served URL,
  so file names can be chosen at runtime. Name such files after the values the
  card produces (`happy.webp` for `<face>happy</face>`).

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
creates a new trial card; set `media.folder` to the old folder name so the
uploads go back there). Contains the target (`trial` or `owned`), the
provider role id, the trial key, the API base, section hashes, uploaded asset
digests and URLs, the media folder, the Lorebook id and the author-asset
version. Commit it if
you want teammates to push to the same card; ignore it if not.
