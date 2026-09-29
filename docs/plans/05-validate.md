# Plan 05 — `card validate`

## Goal

Show the server's pre-publish validation for the card in a folder, in a form
both a person and an agent can act on.

## Behaviour

- `card validate [dir]` resolves the target from `.hearthroom/state.json`
  (trial role or `--to` role). With `--push` it pushes first so the report
  reflects the current files. Without a pushed target it explains that
  validation runs on the server and suggests `card push`.
- Calls `GET /open/v1/role/validate?roleId=…` and prints:
  - status (`pass` / `warning` / `blocker`) and score;
  - blockers, warnings and suggested fixes as bullet lists;
  - the token budget with the per-field limits returned in
    `tokenBudget.limits` and the current counts next to them.
- Exit code: 0 for `pass`, 0 for `warning` (1 with `--strict`), 2 for `blocker`.
- `--json` emits the report unchanged.

## Local pre-checks (no server call)

Only checks that need no server knowledge: required files present, `card.json`
parses and has `formatVersion: 1`, referenced assets exist on disk, Markdown
files are UTF-8. Character limits are **not** checked locally: the server
returns them per card language and they change without a CLI release.

## Tests

- Unit: exit-code mapping, rendering of an empty and a full report from
  fixtures, local pre-checks on a broken folder.
- E2E (env-gated): validate after push returns `pass` for the fixture card.

## Acceptance

- A folder with a missing `welcome.md` fails locally before any request.
- A pushed card shows the server report with the limits it returned.
