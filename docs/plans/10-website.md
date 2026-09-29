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

Level 1 (landing): hero, install control with the agent handoff line (once),
the animated terminal cycling six commands with a one-line description each,
one headline and four reasons with links, install again in a card, footer.

Each locale's copy is written in that language with the words the Hearthroom
app uses (角色卡 / 試玩卡 / 世界書, キャラクターカード / 試用カード, 캐릭터 카드 /
체험 카드), not translated from the English. The register follows
cli.github.com: a plain noun heading, one plain sentence, one "Learn about …
→" link. English is the fallback: the English landing at `/` redirects a
first-time browser to its language (`navigator.languages`, zh-TW/HK/MO →
zh-Hant, other zh → zh-Hans, ja, ko); a language picked in the menu is stored
and wins; agents, crawlers and `curl` run no script and stay on `/`. Landing
pages carry `hreflang` alternates with `x-default` on `/`.
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
| Install | One control like gh's: a method menu (macOS Homebrew, macOS/Linux shell script, Windows PowerShell, Windows Scoop, download), the command, Copy with an icon; the method follows the visitor's OS; current version read at build time | build step |
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

## Measured from cli.github.com (2026-09-29)

Type: headings weight 800 with -0.03em tracking (72px hero, 64px section,
24px reasons, 32px card), 24px muted lead, 14–16px body, 12–14px arrow links,
"View all … commands →" at 24px/800 in the text colour. Install bar: 1px
border, 6px radius, `<details>` menu with 14px options and dividers, 16px
muted monospace command, green Copy with an icon at 16px/600. Terminal:
near-black window with a 1px 10% white border, 16px/24px monospace, white
commands, bold white headline lines, grey secondary, green/red status. The
tight tracking and 800 weight are Latin choices; CJK headings on this site use
normal tracking, weight 700 and a looser line-height.

## Packages

- Homebrew: `hearthroom/homebrew-tap`, cask `hearthroom` (binary plus shell
  completions, quarantine flag cleared). GoReleaser updates it on each release
  when the `HOMEBREW_TAP_TOKEN` repository secret exists; the first cask was
  written by hand for v0.1.5.
- Scoop: `hearthroom/scoop-bucket`, manifest `bucket/hearthroom.json` with
  `checkver`/`autoupdate`; same token, same hand-written first version.

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
- Deploy through CI (`.github/workflows/site.yml`, on pushes that touch the
  site, the manual generator or the command tree) with the Cloudflare token
  and account id as repository secrets; `site/scripts/deploy.sh` does the
  same from a local machine.

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

Site search, a release-notes page (link to GitHub Releases), translated
manual pages.
