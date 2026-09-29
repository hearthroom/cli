---
title: Hearthroom CLI
---

# Hearthroom CLI

`hearthroom` is the command-line client for [Hearthroom](https://hearthroom.club), the open community for AI character cards. It is for authors who work in a terminal and for AI coding agents that have one.

## What it does

- A card is a **folder of plain files**: Markdown for the definition and openings, `card.json` for short fields, `lorebook.json`, `rules.json`, and `assets/` for media. You can keep it in git and hand it to an agent. Format: /guides/card-folder/
- `hearthroom card push` syncs the folder to a **private trial card** on the connected provider, uploads referenced assets once, and sends only changed sections. Trial cards expire three days after the last push; `--create` makes a real private card that stays.
- `hearthroom card validate` shows the provider's pre-publish report with the character limits it enforces.
- `hearthroom card import` converts SillyTavern PNG / JSON / CHARX cards and MMD three-file sets, and reports every field that had no place to go.
- `hearthroom play -m "…" --allow-spend` sends a turn and streams the reply. Only this spends credits, and only with the flag.
- `hearthroom card pull`, `card list`, `search`, `tags`, `author`, `media upload|ls|rm`, `models`, `wallet`, `whoami`, `upgrade`.
- Every command accepts `--json`. `HEARTHROOM_TOKEN` signs in without a browser.

## Install

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/hearthroom/cli/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/hearthroom/cli/main/install.ps1 | iex
```

The installer verifies the release checksum, sets up shell completion, and `hearthroom upgrade` updates in place. Builds for all platforms: https://github.com/hearthroom/cli/releases

## Quick start

```sh
hearthroom auth login
hearthroom card init my-card
hearthroom card push my-card --validate
hearthroom play my-card -m "Hello?" --allow-spend
```

## For agents

Run the loop `card import` → edit files → `card push --validate --json` → `play --json`. Output is stable per command; paid actions need `--allow-spend`; the full manual is at /manual/ and as Markdown at /llms-full.txt.

## Links

- Manual: /manual/
- Card folder format: /guides/card-folder/
- Using it with agents: /guides/agents/
- Source and releases: https://github.com/hearthroom/cli
- Community: https://hearthroom.club · https://discord.gg/C7m85YPHmK
