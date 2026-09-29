# Hearthroom CLI

`hearthroom` is the command-line client for [Hearthroom](https://hearthroom.club),
the open community for AI character cards. Authors, and the AI coding agents
they work with, write cards as plain files, push them to the connected card
provider to try them, validate them, import cards from other tools, play them,
and browse the community, all from a terminal.

- Cards are **folders**: Markdown for long text, one JSON manifest for short
  fields, a Lorebook file, display rules and an `assets/` directory.
  See [docs/card-folder.md](docs/card-folder.md).
- `push` syncs a folder to a **private trial card** in one command, uploading
  assets and rewriting references. Only changed sections are sent.
- Every command takes `--json`, so agents and scripts can pipe the output.
- The CLI is a thin client over the public community API and the provider's
  Open API. It adds no server capability and coexists with the provider's MCP
  server: MCP serves clients without a shell, the CLI serves clients with one.

## Install

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/hearthroom/cli/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/hearthroom/cli/main/install.ps1 | iex
```

The scripts download the release for your platform from
[GitHub Releases](https://github.com/hearthroom/cli/releases), verify it
against the release checksums and put `hearthroom` on your PATH. Later,
`hearthroom upgrade` updates in place. Every release asset carries a build
provenance attestation: `gh attestation verify <file> --owner hearthroom`.

With Go installed you can also build from source:
`go install github.com/hearthroom/cli/cmd/hearthroom@latest`.

## Quick start

```sh
hearthroom auth login                     # opens the provider's sign-in page
hearthroom card init my-card              # card.json, definition.md, welcome.md
$EDITOR my-card/definition.md my-card/welcome.md
hearthroom card push my-card --validate   # private trial card + server report
hearthroom play my-card -m "Hello?" --allow-spend
```

Import a card from another tool, then push it:

```sh
hearthroom card import mira.png           # SillyTavern PNG, JSON or CHARX
hearthroom card import rules.json book.json persona.txt   # MMD three-file set
hearthroom card push Mira
```

## Commands

| Command | What it does |
|---|---|
| `auth login` / `logout` / `status` | Sign in with the browser (OAuth, PKCE, loopback), forget the sign-in, show who you are |
| `card init <dir>` | Create a card folder |
| `card import <files…>` | Convert SillyTavern PNG/JSON/CHARX or an MMD set into a folder; reports anything that did not map |
| `card push [dir]` | Sync to a trial card (default) or an owned card (`--to <id>`, `--create`); uploads assets; `--dry-run`, `--validate` |
| `card validate [dir]` | Show the provider's pre-publish report with its character limits; exit 2 on blockers |
| `card pull <id> [dir]` | Write one of your cards to a folder; `--download` fetches media |
| `card status [dir]` | What the folder is linked to and which sections changed |
| `card list` | Your cards on the provider |
| `card view <id>` | A community card |
| `search [text]` | Browse the board (`--zone`, `--sort`, `--tag`, `--author`) |
| `tags`, `author <handle>` | Community tags and author profiles |
| `media upload <files…>` / `media ls` | Your media library |
| `play [dir] -m "…" --allow-spend` | Send a turn and stream the reply; `--history`, `--stop`; experimental |
| `models`, `wallet`, `whoami` | Provider catalog, balance, identity |
| `upgrade` | Update to the latest release |

Global flags: `--json`, `--api <url>` (provider), `--site <url>` (community),
`--config-dir`. Environment: `HEARTHROOM_TOKEN` (bearer for scripts and CI),
`HEARTHROOM_API`, `HEARTHROOM_SITE`, `HEARTHROOM_CONFIG_DIR`.

## For AI agents

Give the agent a shell and it can drive the whole loop: `card import` →
edit files → `card push --validate --json` → `play … --json`. The folder
format is documented, `--json` output is stable per command, and every paid
action requires `--allow-spend`, so an agent cannot spend credits by accident.
Set `HEARTHROOM_TOKEN` to run without a browser.

## Credits and safety

Starting a conversation, reading history, pushing, validating and browsing are
free. Only `play -m` generates a reply and spends credits on the provider; it
refuses to run without `--allow-spend`. Trial cards are private and expire on
their own; `push --to`/`--create` writes to cards you own. The CLI never
deletes remote content except Lorebook entries it created or pulled itself.

## Development

```sh
go test ./...                 # offline unit tests
go run ./cmd/hearthroom --help
```

End-to-end tests against a local provider are described in
[docs/testing.md](docs/testing.md). Plans and design decisions are in
[docs/plans](docs/plans/00-overview.md).

## License

AGPL-3.0. See [LICENSE](LICENSE).
