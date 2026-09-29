# Plan 06 — `card import`

## Goal

Turn cards from other tools into a card folder, offline, with an explicit
report of anything that did not map. Import never talks to the server; the
author reviews the folder and then runs `card push`.

## Formats

| Input | Detection | Notes |
|---|---|---|
| SillyTavern PNG | PNG signature; `tEXt` chunk `ccv3` (V3) or `chara` (V2), base64 JSON | The image itself becomes `assets/portrait.png` and `card.json.media.portrait`. |
| SillyTavern JSON | `spec` = `chara_card_v3` / `chara_card_v2`, or a bare V1 object with `name` | |
| CHARX | ZIP with `card.json` at the root | Bundled assets under `assets/` are copied; V3 asset URIs (`embeded://`, `ccdefault:`) are rewritten to relative paths. |
| MMD three-file set | Rules export JSON (has `chatVersion`), Lorebook JSON, persona TXT | Files can be given in any order; each is classified by content. A partial set imports what is present and lists the missing parts. |

The parsers are ports of the community site's importers (`png-chunks`,
`tavern`, `mmd`), kept behaviour-compatible so a card imports the same way on
the site and in the CLI. Where the site relies on a browser API, the port uses
the Go standard library.

## Mapping (SillyTavern → folder)

| Source | Destination |
|---|---|
| `name` | `card.json.name` |
| `description` | `definition.md` |
| `personality`, `scenario` | appended to `definition.md` under labelled headings |
| `first_mes` | `welcome.md` |
| `alternate_greetings[]` | `openings/alt-NN.md` |
| `mes_example` | `card.json.talkExample` (split on `<START>`) |
| `system_prompt`, `post_history_instructions` | `card.json.customInstructions` (labelled) |
| `tags[]` | `card.json.tags` |
| `creator_notes` | `README.md` in the folder (not sent to the server) |
| `character_book` | `lorebook.json` (keys, secondary keys → AND gate, constant, enabled, order) |
| `extensions.regex_scripts[]` | `rules.json.rules[]` (find/replace/enabled) |
| everything else | listed in the import report as **not imported** |

## Report

The command prints a table of what was written and a list of source fields it
could not place. With `--json` the same is emitted as structured data. Nothing
is dropped silently.

## Tests

- Fixtures: a V2 PNG, a V3 JSON, a CHARX with one asset, an MMD triple, and
  malformed inputs (truncated PNG chunk, non-card JSON, PNG dropped into the
  MMD set). Expected folders are checked file by file.
- CRC and chunk-length handling are tested against hostile lengths.

## Acceptance

- Each fixture imports to the expected folder; the report lists the known
  unmapped fields for that fixture.
- `card import` followed by `card push` of the V3 fixture validates with no
  blockers on a local provider.
