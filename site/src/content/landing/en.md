---
title: Hearthroom CLI
---

# Hearthroom CLI

`hearthroom` is the command-line client for [Hearthroom](https://hearthroom.club), the open community for AI character cards. It is built for AI coding agents that have a shell and for the authors who run them.

## What it does

- A card is a folder of plain files: Markdown for the definition and openings, `card.json` for short fields, `lorebook.json`, `rules.json`, and `assets/` for media. Format: /guides/card-folder/
- `hearthroom card push` syncs the folder to a private trial card on the connected provider; `--create` makes a card that stays. Details: /guides/trial-cards/
- `hearthroom card validate` returns the provider's pre-publish report with the character limits it enforces.
- `hearthroom card import` converts SillyTavern PNG / JSON / CHARX cards and MMD three-file sets and lists every field it could not place.
- `hearthroom play -m "…" --allow-spend` sends a turn and streams the reply. This is the only command that spends credits, and only with the flag.
- Also: `card pull`, `card list`, `search`, `tags`, `author`, `media upload|ls|rm`, `models`, `wallet`, `whoami`, `upgrade`.
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

More: /guides/install/ · builds for every platform: https://github.com/hearthroom/cli/releases

## A typical loop

```sh
hearthroom auth login
hearthroom card import mira.png            # → folder "Mira", with a report of unmapped fields
hearthroom card push Mira --validate --json
hearthroom play Mira -m "Is the light on tonight?" --allow-spend --json
hearthroom card push Mira --create         # keep it
```

## For agents

Read /llms-full.txt for the complete manual and guides in one file, or any page here as Markdown by appending `.md` to its path (for example /manual/card/push.md). Output shapes are stable per command; errors are one JSON object with a non-zero exit code. Guide: /guides/agents/

## Links

- Manual: /manual/
- Guides: /guides/install/ · /guides/card-folder/ · /guides/trial-cards/ · /guides/agents/
- Source and releases: https://github.com/hearthroom/cli
- Community: https://hearthroom.club · https://discord.gg/C7m85YPHmK
