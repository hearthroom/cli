# Hearthroom CLI

`hearthroom` is the command-line client for [Hearthroom](https://hearthroom.club),
the open community for AI character cards. It lets authors, and the AI coding
agents they work with, write cards as plain files, push them to a provider to
try them, validate them, import cards from other tools, and browse the
community, all from a terminal.

> Status: in development. See [docs/plans](docs/plans/00-overview.md) for the
> roadmap and the decisions behind it.

## Install

Release builds for Linux, macOS and Windows will be published on the
[Releases](https://github.com/hearthroom/cli/releases) page, with an install
script and package-manager formulas. Until the first release, build from source:

```sh
go install github.com/hearthroom/cli/cmd/hearthroom@latest
```

## What it does

```
hearthroom auth login                 sign in to the connected card provider (OAuth, PKCE)
hearthroom card init my-card          start a card folder
hearthroom card import card.png       import a SillyTavern PNG/JSON/CHARX card or an MMD set
hearthroom card push my-card          sync the folder to a private trial card and upload assets
hearthroom card validate my-card      show the server's pre-publish report
hearthroom card pull <roleId> dir     bring an owned card down as files
hearthroom search "keyword"           browse the community board
hearthroom play my-card -m "hi"       send a turn (experimental; spends credits only with --allow-spend)
```

Every command accepts `--json` for machine-readable output.

## Design

The CLI is a thin client over the public Hearthroom community API and the
connected provider's Open API. It adds no server capability and coexists with
the provider's MCP server: MCP serves clients without a shell, the CLI serves
clients with one. Cards live in a documented folder format so they can be
versioned, diffed and edited by agents.

## License

AGPL-3.0. See [LICENSE](LICENSE).
