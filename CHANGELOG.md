# Changelog

## Unreleased

- `lorebook build` and `lorebook check` stop on a second frontmatter block in one file (naming the file and line) instead of merging it into the first entry's content. Subfolders under `worldbook/` are read; files and folders starting with `_` are skipped. Quotes around `name`, `id` and `category` are dropped, and an entry built earlier with a quoted name keeps its Lorebook entry.
- Card variables: `variables.json` (starting values, in the MagVarUpdate `[value, "rule"]` form or as plain values), `variables.md` (rules for the model) and `panel.hbs` (panel skin) are pushed as the card's variables and written by `card pull`. A trial card without `variables.json` has none; an owned card keeps its variables until the file is emptied. `card check` and `card push` check the limits before uploading. `card import` turns SillyTavern `[InitVar]` Lorebook entries (JSON or JSON5) and an AI Fengyue variable panel into these files.
- `card push` accepts `cardFormat: fengyue`, which `card import` writes for AI Fengyue cards.
- `rules.json` `openingMode`: `greeting` (the opening message starts the chat; a tapped opening choice fills the input) or `page` (the player starts from `opening.html` and the choices; a tapped choice is sent, and `welcome.md` may be empty). `card import` writes `page` for AI Fengyue cards; `card pull` writes it when it differs from the default for the card's format. What a tapped choice does no longer depends on `cardFormat`.
- `opening.html`: an opening page shown above the opening of every new chat (never sent to the model), pushed with `rules.json` and written by `card pull`. `card import` writes it from an AI Fengyue card's detailed introduction, and `cardFormat` accepts `fengyue`. A card with opening choices or an opening page may push an empty `welcome.md`.
- `card import` reads AI Fengyue exports (the JSON from the editor's export button), whatever key names that export used: prompt, prefix and suffix text, summary, opening choices, Lorebook, regex rules and built-in CSS land in the folder, and every Fengyue feature without a destination is listed in the report.
- Display rules keep their message targets (AI, user, system) through `card pull` and `card push`.
- `media generate --image <library URL or path>` sends reference images so a character keeps the same face, hair and outfit across images; `media models` lists how many each model takes (REFS). References cost nothing extra.

## 0.10.2 (2026-10-22)

- Disabled Lorebook entries are preserved on push, including their enabled state, matching options and groups.
- Deleting locally removed Lorebook entries from an owned card requires --prune; push lists planned deletions when the flag is absent.

## 0.10.1 (2026-10-21)

- Large Lorebooks follow the provider's operation limit and save progress after each batch; rerunning an interrupted push reuses its card and Lorebook.
- Long entries without author group settings are split without losing whitespace or punctuation; the CLI reports each grouped entry before writing.
- `card push --create` requests a private card. Private and unlisted access are explained separately in the manual.
- Trial pushes check the provider's entry limit before uploading; larger cards can use `--create`.
- Unknown batch outcomes are reconciled with the provider before new entries are sent again.
