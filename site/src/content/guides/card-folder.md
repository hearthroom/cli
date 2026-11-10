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
  intro.md           optional card page introduction in Markdown (roleReadme); not sent to the model
  opening.html       optional opening page: HTML shown above the opening message of every new chat; not sent to the model
  openings/          optional alternate openings, one Markdown file each, in file-name order
    alt-01.md
  lorebook.json      optional Lorebook
  rules.json         optional display rules ("author asset")
  variables.json     optional card variables: values the story keeps exact from turn to turn
  variables.md       optional rules for those values, for the model
  panel.hbs          optional panel skin that shows the values in the chat
  assets/            media referenced by relative path
  AGENTS.md          written by init and import for coding agents; never sent to the provider
  README.md          optional notes; never sent to the provider
  rating.json        content-rating answers for review (written by `card rate`); never sent to the provider
  .hearthroom/
    state.json       sync state written by the CLI
```

`--create` defaults to **unlisted**: anyone with the link can play the latest saved content, including changes that have not passed review. Creating a card does not submit it for review or add it to public listings. To change an owned card's visibility, use `hearthroom card visibility set private my-card`, `set unlisted my-card` or `set public my-card`. Later pushes preserve its visibility. Trial cards stay private.

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
| `summary` | `roleDesc` | one-line public summary for board tiles; the longer card page introduction is `intro.md` |
| `tags` | `roleTag` | |
| `type` | `roleType` | `companion`, `story`, `game`, `generator` |
| `sex` | `roleSex` | stored as-is |
| `playerName` | `userName` | what the card calls the player |
| `nickname` | `nickname` | what `{{char}}` expands to when it differs from `name` |
| `language` | card language | required, one of `zh-Hant`, `zh-Hans`, `en`, `ja`, `ko` (`card push` and `card check` refuse anything else); set when the card is created (a Chinese card can later switch between `zh-Hant` and `zh-Hans`: change it here and push); it picks the leaderboard, the field limits (`en` gets the longer ones) and the Chinese script players convert from |
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
| `agency` | `protect` (never writes the player's part, the platform default), `coauthor` (may write the player character's words, actions and decisions). Older folders may still carry `assist` or `lines`; the server reads them as `protect` and `coauthor`, but a push now asks you to update the value |
| `style` | `default` (the platform writing guide: dialogue carries the scene, narration only does its own job; the platform default), `vivid` (the former default: full sensory description with dialogue and narration side by side), `guided` (lighter guide that defers to your material), `card` (no platform guide; your definition and examples set the voice), `custom` (needs `customStyle`) |
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
- `intro.md` → `roleReadme`: the introduction shown on the public card page,
  like a repository README. It is never sent to the model during chat. The
  file is optional: without it the card page keeps whatever introduction it
  has; an empty file (or one holding only a comment) clears it. `card pull`
  writes it only when the card has an introduction. It is not called
  `readme.md` because on macOS and Windows that is the same file as
  `README.md`, the private notes.
- `openings/*.md` → alternate openings, sorted by file name.
- `opening.html` → the author asset's `openingPage`: an HTML page (styles,
  scripts and handlers allowed) shown at the top of every new chat, above the
  opening message, in its own sandboxed frame on a light background. Players
  see it; the model never does. Buttons in it can call `hearthroom.fill(text)`
  to put a line in the player's input or `hearthroom.send(text)` to send it as
  the player's first message (sent only right after the player clicks; otherwise
  filled), and text the page copies to the clipboard is filled too. It is
  pushed with `rules.json` (no `rules.json`, nothing is sent); without the file
  the stored page is left alone, an empty file clears it, and `card pull`
  writes it when the card has one. A card with opening choices or an opening
  page may leave `welcome.md` empty: the player speaks first. `card import`
  writes it from an AI Fengyue card's detailed introduction.
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

The library accepts images (PNG, JPEG, GIF, WebP, SVG), video (MP4, WebM),
audio (MP3, WAV, Ogg), fonts (WOFF, WOFF2), code (`.js`, `.mjs`, `.wasm`) and
data (`.json`). The type is declared from the file extension and the provider
checks the bytes, so keep the extension true to the content. QuickTime `.mov`
is refused; export the video as MP4 (H.264) first.

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

Lorebook keywords and secondary keywords need `/pattern/flags` syntax for regular expressions (for example, `/chapter[4-6]/`, with flags optional); without the `/…/` delimiters, they are matched as plain text.

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
provider's ids and let later pushes update instead of duplicate. `disabled: true` keeps an entry in the uploaded Lorebook with its enabled state turned off. To delete a CLI-managed entry from an owned card, remove it locally and push with `--prune`. Without that flag, push lists the entries it would delete and stops before writing the Lorebook. Entries created by other tools are kept. Empty entry content is rejected rather than silently omitted.

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

`cardFormat` is empty (MMD-style), `tavern` (styles scoped to messages) or
`fengyue` (imported from AI Fengyue: page-wide styles that follow the light or
dark theme, and tapping an opening choice sends it instead of filling the input).

Each rule may set `cssDialect` to say how the `<style>` in its `replace` is
written: `fengyue` (CSS written for the AI Fengyue chat page: its chat-element
ids such as `#customized-answer-content`, `#ai-chat-input` and `#ai-send-button`
map onto the bubbles, input box and send button; rules starting with `.dark`
apply in dark mode; `body`, `html` and `:root` rules do nothing, as on Fengyue;
buttons keep colour, border and shadow only; `!important` is removed), `tavern`
(every selector prefixed with `.mes_text `) or `mmd` (applied as written).
Without the key the rule follows `cardFormat`. `card import` sets `fengyue` on
the built-in CSS from AI Fengyue.

One card can mix rules with different values. When styles meet on the same
element, the higher level wins:

1. platform defaults;
2. the site's light or dark theme and colours (MMD cards stay dark);
3. `--chat-*` variables, which only change the colours the defaults use;
4. `fengyue` CSS;
5. every other rule's `<style>`: above level 4 whatever the selector weight;
   among themselves normal CSS applies (heavier selector first, then the later
   rule).

To override Fengyue CSS, write a normal `<style>` in another rule; `!important`
is not needed.

`sharedState` declares the polls and counters players can write on this card:

```json
"sharedState": {
  "polls": { "p1-4": { "options": ["a", "b", "c"], "share": true } },
  "counters": { "boss": { "goal": 100000, "maxStep": 50, "perMember": 500, "period": "week" } }
}
```

Keys and option IDs are 1–64 letters, digits, `.`, `_` or `-`, starting with a letter or digit. At most 16 polls and 16 counters; a poll has 2–12 unique options, and `share: true` also allows it on share links. A counter needs `maxStep` (1–1000 per write) and `perMember` (each player's cap per period); `goal` is optional and `period` is `none`, `day` or `week` (UTC+8). `card check` reports a bad declaration. Pushing a file without `sharedState` leaves the server's declaration alone; write `"sharedState": {}` to clear it. Public cards carry it through review with the rest of the card.

### Sharing images and text

Card scripts can offer image and text sharing from a player’s button click. In HTTPS card frames, the host delegates `web-share` and `clipboard-write`: `navigator.share` can receive a prepared image `File` plus text; `navigator.clipboard.write` with `ClipboardItem` can write a PNG, and `navigator.clipboard.writeText` can write a caption. These are browser APIs, not new `sdk` methods or HTTP endpoints.

Prepare the file before the click, then call the action directly in the click handler to retain user activation. Check `navigator.share`, `navigator.canShare({ files: [file] })`, `navigator.clipboard.write` and `ClipboardItem` before offering the corresponding action. Browser, operating system, permissions and receiving app determine support; image and text may not both survive sharing. Catch rejection and cancellation; offer image download and selectable caption text when unavailable or denied, and do not automatically reopen a cancelled share sheet. The host does not delegate `clipboard-read`. Opaque-origin local previews can still reject these APIs; verify the real cross-origin card on the target device.

### Share links that carry a context

Card scripts can create a share link that carries a context code: a friend who opens it gets the same code back, so the card can show “your friend picked this, your turn”. `sdk.share.link(code)` (classic cards: `window.stage.share.link`) returns this card's share link synchronously: the card page `/cards/<number>` with the signed-in player's referral code, plus `?share=<code>` when a code is given. Build the link before the click and pass it to `navigator.share` inside the click handler.

On the friend's side, `sdk.share.context` holds the code; it is `null` when there is none or while the player views a past conversation. A code is 1–64 ASCII letters, digits, `.`, `_` or `-`, starting with a letter or digit; otherwise sandbox cards throw `SHARE_CONTEXT_INVALID` and classic cards get `null`. The platform only delivers the code: it never reaches the model and carries no player data, and anyone can type one, so accept only codes your card defines. Whether the friend already has a save is the card's call (for example `sdk.archive.list()`; a guest has none).

`sdk.share.audience` says who can open the link: `author` (a private card, a new friend cannot), `link` or `public`. With the platform switch off, `sdk.share.enabled` is false, `context` is `null` and `link()` returns the plain card link. `sdk.share.qr(text)` returns `{ size, path }` (an SVG path on a `size` × `size` grid) for your share image. `share:change` fires when the referral code arrives late. Test a context on the draft with `?mode=source&share=<code>`; `card preview` has no share link (`HOST_DENIED`).

### Polls, counters and share responses

The polls and counters declared in `sharedState` are shared by everyone who plays the card. `sdk.shared` (classic cards: `window.stage.shared`) reads and writes them:

- `read(keys)` takes one key or up to 16 and returns `{ [key]: { kind: 'poll', counts, total, mine? } | { kind: 'counter', total, mine?, goal?, reachedAt?, previous? } }`. Undeclared keys are left out. `mine` is the player's own vote or contribution and is missing for guests; `previous` is the previous period's total.
- `vote(key, option)` returns `{ mine }`. Each player can vote or change their vote at most 10 times a day per poll, the first vote included; voting for the same option again does not count.
- `add(key, delta)` adds a whole number from 1 to the counter's `maxStep` and returns `{ total, mine, reachedAt }`. Past `perMember`, only the remainder is added; once nothing more fits it answers `LIMIT_REACHED`.
- `leaderboard(key, { limit })` returns `{ items: [{ rank, name, value }], mine? }`: this period's top contributors (10 by default, at most 50) under their public nickname. Test accounts are never listed. A counter's `total`, `mine` and leaderboard all start again each period.

Anyone can read; writing needs a signed-in player who has played this card (opening a conversation is enough). Nothing is pushed to the card: read again on your own schedule, at most once every 10 seconds. You can try all of this on any card you author, including a new private card and a trial card from `card push`: your numbers are kept apart from the players'.

Share responses build on share links. `sdk.share.tallyLink(code, pollKey)` creates a link with a platform token (`?share=~XXXXXXXX`) for a poll declared with `share: true`. It is asynchronous, so create it before the click; the same code and poll give back the same link, a player can make 20 a day, and a link works for 90 days. A friend who opens it reads the code as `sdk.share.context` and the question as `sdk.share.poll` (`{ key, options }`, otherwise `null`), and answers with `sdk.share.respond(option)`. These answers are kept per link, apart from the poll's own votes. The sharer reads `sdk.share.results()`, a list of `{ context, pollKey, total, counts, updatedAt }` for the links they made: `total` is live, while `counts` appears once 3 friends have answered and updates once a day. The card never learns who shared or who answered. A card may unlock its own content from these numbers; they never grant platform credits.

Both page modes use the same uppercase error codes: `SHARED_DISABLED` (the platform or the author paused it), `NOT_DECLARED`, `OPTION_INVALID`, `NOT_ELIGIBLE` (not played yet, or answering your own link), `UNAUTHORIZED` (signed out), `RATE_LIMITED` (wait and retry), `LIMIT_REACHED` (`err.data.reason` is `goal` or `limit`) and `HOST_DENIED` (an older player page; while viewing a past conversation or a review playtest, only reads work; `tallyLink` on a card that was never submitted or given a visibility, such as a trial card, because nobody else can open it). Author playtests (`?mode=source`) and test accounts write to their own numbers, kept apart from the players'; `hearthroom card state` shows, pauses and resets them.

## variables.json

Card variables are values the story keeps exact from turn to turn: affection,
money, an inventory, the current place, open quests. Every turn the model sees
their current values and your rules for them, and records what the story
changed; the chat page can draw them in a panel. Three files hold them:

| File | Holds |
|---|---|
| `variables.json` | how the values are written (`convention`) and their starting values (`data`) |
| `variables.md` | optional rules for the model: what each value means and what usually changes it |
| `panel.hbs` | optional panel skin: HTML and CSS written in Handlebars, drawn above the chat with the current values; leave it out for no panel |

```json
{
  "convention": "mvu",
  "data": {
    "affection": [50, "+1 to +3 when she is treated kindly, -5 when lied to"],
    "money": [120, "spent when buying, earned from quests"],
    "inventory": {"$meta": {"extensible": true}, "rope": 1},
    "place": "harbor"
  }
}
```

`convention` says how `data` is written:

- `mvu` (the SillyTavern MagVarUpdate form): a value may be written as
  `[value, "rule"]`, and the rule tells the model when and how that value
  changes. An object with `"$meta": {"extensible": true}` can gain and lose
  keys during the chat (`recursiveExtensible` opens every object below it
  too); `"$meta": {"required": ["key"]}` keeps those keys from being removed.
  A plain list is a list. To keep a two-item list whose second item is text
  from being read as `[value, "rule"]`, write it as `[["a", "b"], ""]`.
- `plain` (the default): `data` holds values only and every rule lives in
  `variables.md`. An empty object `{}` can gain keys during the chat; nothing
  else changes shape.

`pathRules` (optional) attaches a rule to a value by its path, such as
`"quests[0]": "done when the letter is delivered"`, where `data` cannot carry
it: a value inside a list, or any value on a `plain` card. `card pull` writes
it only for such rules.

Limits: `data` up to 16 KiB once normalized, nested at most 8 levels, at most
400 keys and 2 KiB per text value; `variables.md` up to 8 KiB; the rules
attached to values up to 8 KiB in total, each naming a value that exists;
`panel.hbs` up to 32 KiB. `card check` and `card push` check all of them
before anything is uploaded. The values travel with every turn, so keep them
to what the story needs to stay exact.

`assets/…` references in `variables.md` and `panel.hbs` are uploaded and
rewritten like those in other files.

The three files are pushed together as one section, replacing the card's
variables. `variables.md` and `panel.hbs` are not sent without
`variables.json`.

- A trial card has no variables unless the folder has `variables.json`.
- An owned card keeps its variables when the folder has no `variables.json`,
  like `lorebook.json` and `rules.json`. To remove them, empty the file
  (leave it with no content) and push.
- `card pull` writes the files when the card has variables, in the form above
  (`mvu` when any value has its own rule, an object was opened with `$meta`
  or a key is required; otherwise `plain`), and removes them when it has none.

`card import` creates the files from a SillyTavern card's MagVarUpdate
initial variables (Lorebook entries marked `[InitVar]`, which then leave the
Lorebook) and from an AI Fengyue card's variable panel (data skeleton, panel
skin and variable prompt).

## rating.json

Submitting a card for review on the community site needs a content rating.
The questionnaire follows Taiwan's game software rating (GSRR); the site
computes the rating from the answers. `hearthroom card rate` asks the questions
(or takes `--answers` from an agent) and writes this file;
`hearthroom card submit` sends the answers with the card.

```json
{
  "answers": {
    "version": 1,
    "topics": {"violence": "violence.bloody", "romance": "romance.dating"},
    "other": "other.none"
  },
  "rating": "PG15",
  "descriptors": ["violence", "romance"]
}
```

`answers` is the source: `version` is the questionnaire version, `topics` holds
one option id for each content type the card has (`{}` when none applies), and
`other` is required. `rating` (G, P, PG12, PG15 or R, low to high) and
`descriptors` are what the site computed when the file was written, kept for
reading. When the site changes the questionnaire it refuses older answers on
submit; run `card rate` again. `card check` reports a missing or malformed
file without failing. The file is not part of the card: `push` never sends it,
and the rating is attached to each review submission instead.

## Sections and sync

The provider stores a trial card in five sections. The CLI hashes the payload
it would send for each section and compares it with the hashes it stored after
the last push, so only changed sections are sent.

| Section | Built from |
|---|---|
| `card` | `card.json` (except `prologue` and `media`, which are resolved) + `definition.md` + `intro.md` |
| `welcome` | `welcome.md`, `openings/*.md`, `card.json.prologue` |
| `worldbook` | `lorebook.json` |
| `authorAsset` | `rules.json` |
| `state` | `variables.json` + `variables.md` + `panel.hbs` |

## .hearthroom/state.json

Written by the CLI. Keep this file when resuming an interrupted push: it links the
folder to its card and Lorebook and records completed batches, pending operations
and learned entry IDs. Removing it loses that link and may create another card or
Lorebook on the next push. It also contains section hashes, uploaded asset digests
and URLs, the media folder and the author-asset version. Share it only with
teammates who should update the same card; pending operations can contain card text.

Lorebook writes read the provider's limits from `/open/v1/worldbook/limits`.
The current provider allows 200 operations per request and 1000 entries per trial
card. Entry content limits are 4000 characters for Chinese, 6000 for Japanese and
Korean, and 12000 for English; a bound book's detail response provides its actual
`entryContentMax`. Providers without limit discovery use the published v1 limits.

A long entry without `matchOptions.groupId` or `groupOrder` is split for upload at
paragraph or sentence boundaries, with shared `groupId` and increasing `groupOrder`.
Joining the parts exactly restores the original text; the local file stays unsplit.
Existing group settings are preserved, so an oversized author-grouped entry must
be shortened by its author. Grouped parts count separately toward the trial limit.


## Author history and offline preview

Cards can probe `sdk.history?.capabilities().available` (sandbox) or
`stage.history?.capabilities().available` (classic) before reading the complete
owned conversation through `history.list`. Continue every page at the same
revision, and treat stale, reset or unavailable results as unknown state rather
than empty history. `history.onChange` is independent of viewport mount/unmount
and returns an unsubscribe function. Only successful complete AI messages are
safe state checkpoints; optional content excludes thinking, streaming bodies
and moderation-withheld text.

Each history item includes `replyTo`. For an AI message with a verified link,
it is the opaque `id` of the player message it answers in the same conversation's
history. Player messages, openings and messages without a verified link return
`null`. Older messages and historical copies in a fork can also return `null`.
Treat that as an unknown relationship; do not infer one from adjacent items.
These opaque IDs differ from the raw message IDs in `/conversation/messages`.

| Operation | Effect on `replyTo` |
|---|---|
| Send | The AI item points to the new player item's `id`. |
| Regenerate or edit and resend | The replacement AI item points to the replacement player item's new `id`. |
| Manually edit an AI reply | The existing link stays the same. |
| Backward or delete | Re-read history after invalidation; retained links require the player item to still exist. |
| Resume paused agent preparation | The new AI item keeps the original player item's `id`. |

In `history.onChange`, `add` and `update` changes carry the currently verifiable
`replyTo`; `remove` changes carry `null`. Events invalidate cached history, so
re-read `history.list` at its current revision. An event's link is not a snapshot
of its older revision, and it can be `null` if either item has since disappeared.

The local preview injects synthetic messages and has no authenticated Provider
history. It does not prove complete live membership or cross-device change
handling; cards must keep a readable fallback when this capability is missing.
Verify actual delete, truncate and rewrite recovery through an isolated signed-in
browser against the candidate Provider and stage before publishing.
