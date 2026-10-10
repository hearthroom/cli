---
slug: "card/rate"
command: "hearthroom card rate"
short: "Answer the content-rating questionnaire and write rating.json"
parent: "card"
---

# hearthroom card rate

Submitting a card for review needs a content rating. The questionnaire follows
Taiwan's game software rating (GSRR): pick which content types the card has, then
for each one the closest option that is not milder than the actual content, then
one question about anything else. The site computes the rating (G, P, PG12, PG15,
R) from the answers; the CLI only sends them.

In a terminal the command asks the questions. Without one, read the questionnaire
with --questions --json, write the answers as JSON and pass them with --answers
(a file, or - for stdin):

  {"version": 1, "topics": {"violence": "violence.bloody"}, "other": "other.none"}

topics lists only the content types the card has ({} for none); other is required.
The answers are checked by the site and written to rating.json in the folder,
which card submit sends with the card. rating.json never goes to the provider.
When the folder is linked to a card you own and you are signed in, the answers
are also saved as the website's rating draft for that card.

The questionnaire language is --locale, else LC_ALL / LC_MESSAGES / LANG, else
the card's language, else English. Rating is free and never submits the card.

## Usage

```
hearthroom card rate [dir] [flags]
```

## Options

```
      --answers string   answers JSON file, or - for stdin, instead of asking
      --locale string    questionnaire language: zh-Hant, zh-Hans, en, ja, ko
      --questions        print the questionnaire (with --json, as the site returns it) and exit
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, check, push, validate, render, preview, pull, import, list, rate, submit
