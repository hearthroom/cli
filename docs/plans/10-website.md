# Plan 10 — cli.hearthroom.club

## Goal

A small website, modelled on cli.github.com, that gets an author from "what is
this" to a working `hearthroom` in one screen, and gives both people and AI
agents the complete manual. Served on `cli.hearthroom.club`,
`cli.sukisuki.ai` and `cli.sukisuki.chat`.

## Who this is for, and what that does to the page

The people who reach this page are (1) authors who already run a coding
agent and want to hand it the tool, (2) authors comfortable in a terminal,
and (3) agents themselves, which arrive through `llms.txt`. The first two
decide in seconds; the third needs everything and reads Markdown.

So the site has two densities from one source. The HTML landing is sparse:
about the same word count as cli.github.com (one hero line, one install
control, one animated demo, three one-sentence reasons). The `.md` twins
and `llms-full.txt` are complete. Nothing is written twice; the guides and
manual hold every detail that was cut from the landing.

Copy rules for the landing, applied line by line: one job per block, one
sentence per block, the reader's action before the feature, no numbers
(limits and counts live in the manual), no adjective that does not change
the install decision, and one link per block to the page that goes deeper.

Level 1 (landing): hero, install control with the agent handoff line, the
animated terminal cycling six commands with a one-line description each,
three reasons with links, install again, footer.
Level 2 (guides): installing, the card folder format, trial cards versus
real cards, using it with agents. Level 3 (manual): every command, generated.

Demo data is invented (cards, authors, ids), never a real account or card.
The search transcript follows the interface language because the board is
split into language zones; the rest of the terminal is English because the
CLI speaks English.

## Information architecture (after cli.github.com)

| Section | Content | Source |
|---|---|---|
| Nav | Manual · Guides · Releases (GitHub) · GitHub · Discord · language · theme | static |
| Hero | "Your cards, from the terminal." + one line + primary install command | hand-written, 5 locales |
| Install | Tabs macOS / Linux / Windows: script one-liner, Homebrew/Scoop (when the tap exists), from source; current version read from the latest release at build time | build step |
| Features | One command per block with real output captured in this session: `card import`, `card push --validate`, `play -m … --allow-spend`, `card pull`, `search`, `media rm` | hand-written |
| Agents | Claude Code / Codex loop, `--json`, `HEARTHROOM_TOKEN`, `--allow-spend` | hand-written |
| Manual | Every command with flags, generated from the binary's own help | `cmd/gendocs` (cobra/doc) |
| Guides | Card folder format, installing, using with agents, testing | `docs/*.md` from this repo |
| Footer | AGPL, GitHub, Discord, hearthroom.club | static |

Landing and guides exist in the five site locales (`en`, `zh-Hant`, `zh-Hans`,
`ja`, `ko`) under `/{locale}/`, English at `/`. The manual is English only
because the CLI's help text is English; it is generated, not translated.

## One source, two readers

Everything is Markdown first. The build renders HTML for people and emits, for
every page, a `.md` twin at the same path (`/manual/card/push/` ↔
`/manual/card/push.md`), plus `/llms.txt` (an index with one line per page)
and `/llms-full.txt` (the whole English manual and guides concatenated). Each
HTML page carries `<link rel="alternate" type="text/markdown">`. This mirrors
what hearthroom.club ships for its own docs. No `Accept` negotiation in v1;
static twins are enough and cacheable.

The manual cannot drift from the binary: `cmd/gendocs` runs `cobra/doc` over
the real command tree at build time, so the site says exactly what `--help`
says for that release.

## Hosting

- `site/` in this repository: Astro (static output), Vitest for the twin and
  llms files.
- One Cloudflare Worker with static assets (`site/wrangler.toml`), routes
  `cli.hearthroom.club/*`, `cli.sukisuki.ai/*`, `cli.sukisuki.chat/*` as zone
  routes. The main site's Worker owns `*.<zone>/*` wildcards; a more specific
  route wins, and `custom_domain` cannot coexist with those wildcards. The
  wildcard proxied DNS records already exist, so no DNS change.
- Canonical host `cli.hearthroom.club`; the two sukisuki hosts serve the same
  content with a canonical link, like the main site's aliases.
- Deploy from a local machine with the logged-in `wrangler`
  (`site/scripts/deploy.sh`: build the CLI, generate the manual, `astro build`,
  `wrangler deploy`). CI deploy needs a Cloudflare API token as a repository
  secret; follow-up.

## Theme

Hearthroom's `tokens.css` and `themes.css` (same organisation, same licence)
so light and dark match the site; the same icon; system font stack; terminal
blocks with a copy button.

## Verification

- `astro build` clean; vitest pins twin paths and `llms-full.txt` contents.
- `wrangler deploy --dry-run`, then the real deploy.
- `curl` on all three hosts: `/`, one manual page, its `.md` twin,
  `/llms.txt`, `/llms-full.txt`.
- Regression on the main Worker: `hearthroom.club/` and a sandbox host still
  answer as before.
- A browser pass of the landing in light and dark.

## Out of scope for v1

Site search, a release-notes page (link to GitHub Releases), CI deploy,
translated manual pages.
