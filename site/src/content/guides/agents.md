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

## Signing in

`hearthroom auth login` prints a one-time code and an address. The person opens the address on any device, signs in and enters the code, so it works from SSH sessions and cloud sandboxes. A plain `auth login` waits until they approve, which is no use to an agent that only sees output once a command exits. Split the wait instead:

```sh
hearthroom auth login --no-wait --json
# {"user_code": "BCDF-GHJK", "verification_uri": "…/device",
#  "verification_uri_complete": "…/device?user_code=BCDF-GHJK", "expires_in": 900, "interval": 5}
# Give the person verification_uri_complete (or verification_uri and the code). Once they approve:
hearthroom auth login --resume --json
```

`--resume` prints the same result as a normal login. It waits up to `--timeout` (default 5 minutes); if the person has not approved by then it exits non-zero and keeps the request, so run `--resume` again. After `expires_in` seconds, or if the person denies it, start over with `--no-wait`. If the provider does not offer sign-in with a code, `--no-wait` fails and says so.

Tokens are stored under the user config directory; set `HEARTHROOM_CONFIG_DIR` to isolate an agent's session. To skip sign-in entirely, set `HEARTHROOM_TOKEN` to a provider access token.

## Seeing the result without a browser

`card render` asks the provider to run the card's display rules over one opening with the same engine the play page uses. The JSON carries the text the renderer receives (`rendered`), each rule's outcome (`applied`, `unmatched`, `disabled`, `empty`, `rolled_back` with a reason), a static scan (tags, `hc-*` components, scripts, inline handlers, external URLs) and `report.unsupported`: author-API identifiers the card's chat page does not provide, each with a `hint` (a classic-page card calling the sandbox `sdk`, for example, should switch to the sandbox page). Layout, contrast and Markdown are only visible on the play page; `previewUrl` points there. An agent with a browser should open it at a phone and a desktop width.

## Writing a good card, not just moving one

The CLI moves files and brings evidence back; it does not know what makes a card good. Two open-source skill sets do:

- [hearthroom/skills](https://github.com/hearthroom/skills) teaches the writing craft for Hearthroom cards: premise, character, Lorebook, openings, voice, state, diagnosis and iteration, and it drives this CLI for every check. Install it into Claude Code or Codex, or point any agent at its router skill.
- [yofengi/tavern-mmd](https://github.com/yofengi/tavern-mmd) builds status bars, themes, floating panels and full custom chat pages for SillyTavern and Meimo Island (MMD). Hearthroom's sandbox chat page runs the same author API as MMD's new-style sandbox, so its `/mmdsandbox` output imports with `hearthroom card import` and runs as is; `card render --json` confirms it (only `sdk.vars` should appear under `report.unsupported`).

## Spending is opt-in

Starting a conversation, reading history, pushing, validating, rendering, importing and browsing are free. Only `play -m` generates a reply and spends credits on the provider, and it refuses to run without `--allow-spend`. An agent cannot spend credits by accident.

## A typical loop

```sh
hearthroom card import mira.png                 # SillyTavern PNG → folder, with a report of unmapped fields
$EDITOR Mira/definition.md Mira/welcome.md
hearthroom card push Mira --validate --json     # private trial card + the provider's report
hearthroom card render Mira --json              # the opening after the display rules, each rule's outcome, a static scan
hearthroom play Mira -m "Who are you?" --allow-spend --json
hearthroom card push Mira --create              # keep it: a real private card that shows on the site
```

## The card folder

Long text lives in Markdown files, short fields in `card.json`, the Lorebook in `lorebook.json`, display rules in `rules.json`, media under `assets/`. The format is versioned and documented in [Card folder format](/guides/card-folder/). Unknown keys in `card.json` survive a round trip, and `.hearthroom/state.json` records what the folder is synced to.
