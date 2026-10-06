<p align="center">
  <a href="https://hearthroom.club"><img src="https://raw.githubusercontent.com/hearthroom/hearthroom/main/web/public/icons/icon-192.png" width="96" alt=""></a>
</p>

<h1 align="center">Hearthroom CLI</h1>

<p align="center">Write, test and publish AI character cards from your terminal, or from an AI agent's.</p>

<p align="center">
  <a href="https://github.com/hearthroom/cli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/hearthroom/cli"></a>
  <a href="https://github.com/hearthroom/cli/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/hearthroom/cli/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://discord.gg/C7m85YPHmK"><img alt="Discord" src="https://img.shields.io/badge/Discord-join-5865F2?logo=discord&logoColor=white"></a>
  <a href="LICENSE"><img alt="License: AGPL-3.0" src="https://img.shields.io/github/license/hearthroom/cli"></a>
</p>

`hearthroom` is the command-line client for [Hearthroom](https://hearthroom.club), the open community for AI character cards. A card is a folder of plain files. You edit it with any editor or any coding agent, push it to a private trial card to play it, validate it against the provider's rules, and browse the community, without leaving the shell.

If you want the web editor, use [hearthroom.club](https://hearthroom.club). The CLI is for people and agents who already have a terminal.

## Install

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/hearthroom/cli/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/hearthroom/cli/main/install.ps1 | iex
```

The scripts download the build for your platform from [GitHub Releases](https://github.com/hearthroom/cli/releases), check it against the release checksums, and put `hearthroom` on your PATH. Set `HEARTHROOM_INSTALL_DIR` to choose the directory and `HEARTHROOM_VERSION` to pin a release.

<details>
<summary>Other ways to install</summary>

- **Homebrew**: `brew install hearthroom/tap/hearthroom` (once the tap is published)
- **Scoop**: `scoop bucket add hearthroom https://github.com/hearthroom/scoop-bucket && scoop install hearthroom` (once the bucket is published)
- **From source** (Go 1.26+): `go install github.com/hearthroom/cli/cmd/hearthroom@latest`
- **Manual**: download the archive for your platform from the [latest release](https://github.com/hearthroom/cli/releases/latest), unpack it and put the binary on your PATH. Every asset has a build provenance attestation: `gh attestation verify <file> --owner hearthroom`.

</details>

The installer also sets up tab completion for your shell (bash, zsh or fish; `hearthroom completion install` does the same later, `completion uninstall` removes it). `hearthroom` checks for a newer release at most once a day and tells you which command upgrades it. `hearthroom upgrade` updates in place; `HEARTHROOM_NO_UPDATE_NOTIFIER=1` silences the check.

## Quickstart

```sh
hearthroom auth login                      # sign in with a one-time code (works over SSH)
hearthroom card init my-card               # card.json, definition.md, welcome.md, assets/
$EDITOR my-card/definition.md my-card/welcome.md
hearthroom card push my-card --validate    # private trial card + the provider's report
hearthroom card render my-card             # the opening after your display rules, plus the play link
hearthroom play my-card -m "Hello?" --allow-spend
```

Coming from SillyTavern or MMD? Import first, then push:

```sh
hearthroom card import mira.png                          # PNG, JSON or CHARX
hearthroom card import rules.json book.json persona.txt  # MMD three-file set
hearthroom card push Mira
```

The import report lists every field that had no place to go. Nothing is dropped silently.

## How it works

- **Cards are folders.** Long text in Markdown (`definition.md`, `welcome.md`, `openings/*.md`), short fields in `card.json`, the Lorebook in `lorebook.json`, display rules in `rules.json`, media under `assets/`. The format is versioned and documented in [docs/card-folder.md](docs/card-folder.md), so you can keep cards in git and let agents edit them.
- **Push is one command.** Assets referenced from the folder are uploaded once and rewritten to their served URLs; the card is written in sections and only changed sections travel.
- **Trial cards are a sandbox.** By default `push` writes to a private *trial card*: it expires three days after the last push, the provider keeps at most five per account (`--evict` frees the oldest), and the website's card inventory does not list it, though the play link works. When the card is ready to keep, `push --create` makes a real private card that shows up on the site and can be submitted for review, and later pushes update it; `push --to <id>` writes into a card you already own, for example one you pulled.
- **Validate is the provider's word.** Character limits and quality checks come from the server per card language; the CLI shows them next to your counts and exits non-zero on blockers.
- **Play is opt-in.** Starting a conversation and reading history are free. Sending a message spends credits, so it runs only with `--allow-spend`, streams the reply, and reports what was settled.
- **Everything speaks JSON.** Add `--json` to any command for stable machine output; `play --json` emits one event per line.

## Commands

| Command | Purpose |
|---|---|
| `auth login` · `auth logout` · `auth status` | Sign in with a one-time code you approve on any device (`--no-wait` / `--resume` for agents; browser redirect on a loopback port when the provider has no codes), sign out, who am I |
| `card init <dir>` | Create a card folder |
| `card import <files…>` | SillyTavern PNG / JSON / CHARX or an MMD set → folder, with a report |
| `card push [dir]` | Sync to a trial card (default) or an owned card; `--dry-run`, `--validate`, `--evict` |
| `card validate [dir]` | The provider's pre-publish report; `--push` first, `--strict` fails on warnings |
| `card check [dir]` | Local checks against the sandbox contract: rules, markers, sdk use; `--replay file` measures the status protocol on real replies |
| `card render [dir]` | An opening after the display rules, each rule's outcome and a static scan; `--opening N`, `--html file`, `--push` |
| `card preview [dir]` | The folder in the site's real sandbox shell on a local port, offline; `--open`, `--port`, `--shell dir` |
| `card pull <id> [dir]` | One of your cards → folder; `--download` fetches media |
| `card status [dir]` | Linked card and changed sections |
| `lorebook build [dir]` · `lorebook check [dir]` | `worldbook/*.md` → `lorebook.json`; drift and keyword collisions |
| `card list` · `card view <id>` | Your cards on the provider; a community card |
| `search [text]` · `tags` · `author <handle>` | Browse the board: `--zone`, `--sort`, `--tag`, `--author` |
| `media upload <files…>` · `media ls` | Your media library |
| `play [dir] -m "…" --allow-spend` | Send a turn and stream the reply; `--history`, `--stop`, `--model`, `--json` |
| `models` · `wallet` · `whoami` | Provider catalog, balance, identity |
| `upgrade` | Update to the latest release (`--check` only reports) |

Global flags: `--json`, `--api <url>` (provider), `--site <url>` (community site), `--config-dir`.
Environment: `HEARTHROOM_TOKEN` (bearer for scripts and CI, skips the browser), `HEARTHROOM_API`, `HEARTHROOM_SITE`, `HEARTHROOM_CONFIG_DIR`, `HEARTHROOM_NO_UPDATE_NOTIFIER`.

## Using it with an AI agent

Point Claude Code, Codex or any agent with a shell at a card folder and it can run the whole loop: `card import` → edit files → `card check` → `card push --validate --json` → `card render --json` → `card preview` → `play --new-session --json` → `card check --replay` → edit again. `card render` runs the display rules with the same engine as the play page and reports each rule's outcome, so an agent without a browser still sees what the player will. The folder format is documented, the JSON output is stable per command, and every paid action needs `--allow-spend`, so an agent cannot spend credits by accident. For unattended runs set `HEARTHROOM_TOKEN`.

Agents should start at [sukisuki.ai/llms.txt](https://sukisuki.ai/llms.txt): it says what a card is, which actions need sign-in or spend credits, and where the Markdown sources of the authoring guide and API reference are. [llms-full.txt](https://sukisuki.ai/llms-full.txt) has all of it in one file.

## Provider and community

Hearthroom is provider-neutral. The CLI reads the provider API base from `--api`, `HEARTHROOM_API` or its config, and learns capabilities such as relative media paths and file limits from the provider's responses. For the default provider it signs in through a client issued to the CLI by the community application, so you see the same cards as on the website; for any other provider it registers itself dynamically, and that provider decides what such a client may see. The community site's public endpoints need no sign-in; member endpoints reuse the same sign-in as the provider. Nothing here adds server capability: the CLI is a client of the documented [Hearthroom developer APIs](https://hearthroom.club/developers).

## Development

```sh
go test ./...                 # offline unit tests
go run ./cmd/hearthroom --help
```

End-to-end tests against a local provider are described in [docs/testing.md](docs/testing.md). The plans behind each part live in [docs/plans](docs/plans/00-overview.md). Releases are built by GoReleaser from tags and carry build provenance attestations.

## License

[AGPL-3.0](LICENSE)
