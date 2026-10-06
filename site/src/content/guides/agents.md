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
# {"user_code": "BCDF-GHJK", "verification_uri": "…/device", "expires_in": 900, "interval": 5}
# Give the person verification_uri and the code; they type the code on that page. Once they approve:
hearthroom auth login --resume --json
```

`--resume` prints the same result as a normal login. It waits up to `--timeout` (default 5 minutes); if the person has not approved by then it exits non-zero and keeps the request, so run `--resume` again. After `expires_in` seconds, or if the person denies it, start over with `--no-wait`. If the provider does not offer sign-in with a code, `--no-wait` fails and says so.

Tokens are stored under the user config directory; set `HEARTHROOM_CONFIG_DIR` to isolate an agent's session. To skip sign-in entirely, set `HEARTHROOM_TOKEN` to a provider access token.

## Seeing the result without a browser

`card check` runs locally, without sign-in: it reads `rules.json`, the definition, the output contract, the Lorebook and the openings against the sandbox author contract embedded in the build, and reports invalid or empty-matching patterns, a replacement over 128 KiB, sdk capabilities or events that do not exist, attributes the sanitizer strips, a marker a rule draws that the model is never told to write, and more; `--replay` measures the `[status]` block on real replies. `lorebook build` writes `lorebook.json` from one Markdown file per entry under `worldbook/`, and `lorebook check` finds keywords that fire several entries at once.

`card render` asks the provider to run the card's display rules over one opening with the same engine the play page uses. The JSON carries the text the renderer receives (`rendered`), each rule's outcome (`applied`, `unmatched`, `disabled`, `empty`, `rolled_back` with a reason), a static scan (tags, `hc-*` components, scripts, inline handlers, external URLs) and `report.unsupported`: author-API identifiers the card's chat page does not provide, each with a `hint` (a classic-page card calling the sandbox `sdk`, for example, should switch to the sandbox page). Layout, contrast and Markdown are not in that report. `card preview` serves the folder to the site's own sandbox shell on a local port with the CLI as a fake host: the real sdk, sanitizer, Markdown and event order, sample replies streamed from `preview/replies.md`, five sizes, both themes, and `?rules=off` to read the card as plain text. `card preview --check` does the looking for an agent without a browser: headless Chrome drives the same page through a fixed set of states (phone and desktop, dark and light, rules on and off, first and last sample) and leaves `preview/shots/` with a screenshot per state, a contact sheet of all of them, and `findings.json` with what a picture does not show: how long after the reply the status panel was drawn, sideways overflow at phone width, choice-button count, text under 12px, the card scripts' console errors. `--from-history` streams real replies from a `play --history` file. An agent with a browser can also open the page itself, or `previewUrl` from the render report.

## Writing a good card, not just moving one

The CLI moves files and brings evidence back; it does not know what makes a card good. The open-source skill set [hearthroom/skills](https://github.com/hearthroom/skills) does: premise, character, Lorebook, openings, voice, state, presentation, diagnosis and iteration, and it drives this CLI for every check. Install it into Claude Code or Codex, or point any agent at its router skill.

For the screen (a status panel drawn from a block the model writes, choice buttons, a theme with a dark and a light side, a settings drawer) use that toolkit's own sandbox kit (`assets/sandbox-kit`): `build.mjs` turns one config into display rules and emits the model-side protocol from the same schema. Then `hearthroom card check` reads the folder against the sandbox contract (rules, markers, sdk use, and with `--replay` the status protocol's health on real replies), and `hearthroom card preview --check` drives the folder through the site's real sandbox shell headless and leaves screenshots, a contact sheet and findings an agent can read (`--open` for a browser). Cards written for SillyTavern or for Meimo Island's older chat page still import with `card import`; for those platforms themselves, [yofengi/tavern-mmd](https://github.com/yofengi/tavern-mmd) remains the reference. The chat page itself is open source (`hearthroom/moonstage`): the skills' sandbox contract is generated from its `src/sandbox/`, and the skills' facts sheet names the files to read, and the cautions, when a fact is missing.

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
