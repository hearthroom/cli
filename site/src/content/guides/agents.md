---
title: Using it with AI agents
---

# Using it with AI agents

`hearthroom` was built for the loop an author runs with a coding agent: import or edit a card as files, push it, read the validation report, play a turn, edit again. Any agent with a shell can drive it, including Claude Code and Codex.

## Give the agent the manual

The complete manual is generated from the binary and served as Markdown:

- `/llms.txt` lists every page;
- `/llms-full.txt` is the whole English documentation in one file;
- every page on this site has a `.md` twin at the same path, for example `/manual/card/push.md`.

Point the agent at `https://cli.hearthroom.club/llms-full.txt`, or run `hearthroom <command> --help` locally; both say the same thing for the installed version.

## Machine-readable output

Add `--json` to any command. Output shapes are stable per command; error output is a single object `{ "error": "...", "detail": { ... } }` with a non-zero exit code.

```sh
hearthroom card push my-card --validate --json | jq '.validation.status'
hearthroom play my-card -m "Hello?" --allow-spend --json   # one JSON event per line
```

## Sign in without a browser

Set `HEARTHROOM_TOKEN` to a provider access token and no browser is needed. Interactive `hearthroom auth login` stores tokens under the user config directory; set `HEARTHROOM_CONFIG_DIR` to isolate an agent's session.

## Spending is opt-in

Starting a conversation, reading history, pushing, validating, importing and browsing are free. Only `play -m` generates a reply and spends credits on the provider, and it refuses to run without `--allow-spend`. An agent cannot spend credits by accident.

## A typical loop

```sh
hearthroom card import mira.png                 # SillyTavern PNG → folder, with a report of unmapped fields
$EDITOR Mira/definition.md Mira/welcome.md
hearthroom card push Mira --validate --json     # private trial card + the provider's report
hearthroom play Mira -m "Who are you?" --allow-spend --json
hearthroom card push Mira --create              # keep it: a real private card that shows on the site
```

## The card folder

Long text lives in Markdown files, short fields in `card.json`, the Lorebook in `lorebook.json`, display rules in `rules.json`, media under `assets/`. The format is versioned and documented in [Card folder format](/guides/card-folder/). Unknown keys in `card.json` survive a round trip, and `.hearthroom/state.json` records what the folder is synced to.
