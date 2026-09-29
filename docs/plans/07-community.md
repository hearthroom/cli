# Plan 07 — Community and account commands

## Goal

Cover the read-only community surface and the small account verbs an author
uses between edits.

## Commands

| Command | Endpoint | Auth |
|---|---|---|
| `search <text> [--zone zh\|en\|ja\|ko\|all] [--sort new\|top] [--tag t]...` | `GET <site>/v1/cards` | none |
| `card view <id>` | `GET <site>/v1/cards/{id}` | optional |
| `tags [--q text]` | `GET <site>/v1/tags` | none |
| `author <handle>` | `GET <site>/v1/authors/{handle}` | none |
| `card list [--q text]` | `GET <api>/open/v1/role/mine` | required |
| `whoami` | `GET <api>/open/v1/me`, then `GET <site>/v1/me` | required |
| `wallet` | `GET <api>/open/v1/me/wallet` | required |
| `models` | `GET <api>/open/v1/models` | required |

Human output is a compact table; `--json` returns the API payload unchanged so
agents can pipe it. Pagination uses `--limit/--offset` and prints `hasNext`.

## Not in this plan

Comments, favorites, follows and notifications exist on `/v1` but are social
actions, not authoring; they can be added later without design work.
Submitting a card for community review has no documented public endpoint yet
and is tracked as a follow-up in the overview.

## Tests

- Unit: table rendering from fixture payloads, zone/sort flag validation.
- E2E: `card list` and `whoami` against the local provider; `search` against
  a recorded response (the site is not run locally).

## Acceptance

- `hearthroom search "school" --zone en --json | jq '.items[0].id'` works
  anonymously against the live site.
