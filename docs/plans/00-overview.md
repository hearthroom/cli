# Hearthroom CLI — plan overview

`hearthroom` is a command-line client for authors who write, test and publish
character cards on [Hearthroom](https://hearthroom.club). It is built for two
kinds of users: people working in a terminal, and AI coding agents that already
have a shell (Claude Code, Codex, CI jobs).

## What the CLI is, and is not

- It is a **thin client** over two existing public contracts:
  - the connected card provider's `/open/v1` API (card content, Lorebooks,
    media, conversations, credits; OAuth 2.1 with PKCE);
  - the community site's `/v1` API (browse, search, comments, favorites).
- It **does not** add server capabilities. Every verb maps to an endpoint that
  exists today. Anything that needs a new server surface is listed as a
  follow-up, not built here.
- It **does not replace MCP**. MCP serves clients without a shell (web, mobile
  chat UIs). The CLI serves clients with one. Both stand on the same contracts.
- Local files are the source of truth. A card is a folder you can diff, version
  and hand to an agent. `push` syncs it to the provider; `pull` brings it back.

## Provider neutrality

Hearthroom is provider-agnostic. The CLI reads the API base from configuration
(`--api`, config file, or `HEARTHROOM_API`), never hardcodes a provider host, and
reads capabilities (`relativePaths`, `libraryPrefix`, limits) from responses
instead of assuming them.

Vocabulary in commands and help follows the public terminology charter: the
configuration asset is an **agent configuration** (a "card" in community
language), and its entry-based background information is a **Lorebook**. Wire
field names stay as the API defines them.

## Phases

| # | Plan | Delivers |
|---|---|---|
| 1 | [01-scaffold](01-scaffold.md) | Go module, command tree, config, `--json`, CI on three OS, GoReleaser snapshot |
| 2 | [02-auth](02-auth.md) | `auth login/logout/status`, PKCE loopback flow, dynamic client registration, token refresh, `HEARTHROOM_TOKEN` |
| 3 | [03-card-folder](03-card-folder.md) | The folder format (`formatVersion: 1`), `card init`, `card pull` |
| 4 | [04-push-and-media](04-push-and-media.md) | `card push` (trial card by default, owned card with `--to`), asset upload with relative paths and reference rewriting |
| 5 | [05-validate](05-validate.md) | `card validate` against the server report |
| 6 | [06-import](06-import.md) | `card import` for SillyTavern PNG / JSON / CHARX and the MMD three-file set |
| 7 | [07-community](07-community.md) | `search`, `card view`, `whoami`, `card list` |
| 8 | [08-play](08-play.md) | `play` (experimental): start a conversation and send a turn, refusing paid actions unless `--allow-spend` |
| 9 | [09-release](09-release.md) | Tagged releases, `install.sh`, checksums, build provenance attestations, Homebrew tap and Scoop bucket |

## Verification bar

- Unit tests with fixtures for importers, folder format round-trip and PKCE.
- An end-to-end Go test against a locally running provider, gated by
  `HEARTHROOM_E2E_API`, that covers login (headless), push, validate and pull.
- CI green on Linux, macOS and Windows; `hearthroom version` runs on Windows.
- `goreleaser release --snapshot` builds all six targets.
- `install.sh` exercised from an actual release asset.

## Decisions taken in these plans

- Language: Go. Single static binary, no runtime, cross-compiles in CI.
- License: AGPL-3.0, matching the other Hearthroom repositories.
- CLI output language: English for v0.1. Localized READMEs are a follow-up.
- Scopes requested: `profile.read email.read role.read role.write chat.play`.
  `email.read` only identifies the signed-in account in messages. Not
  `referral` or `model.invoke`.
- Trial cards are the default push target: they are private, auto-expiring
  and keyed by a local id, which is exactly the "edit locally, try it now" loop.
- Spending credits is opt-in per invocation (`--allow-spend`).
