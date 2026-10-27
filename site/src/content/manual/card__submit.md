---
slug: "card/submit"
command: "hearthroom card submit"
short: "Submit the linked card for review on the community site, with its content rating"
parent: "card"
---

# hearthroom card submit

Sends the card the folder is linked to, as last pushed, for review on the
community site, together with the answers in rating.json (see card rate).
Only a card you own can be submitted. A trial card cannot; to keep it first,
run "hearthroom card push --create". The new card defaults to unlisted:
anyone with its link can play the latest saved content before review.
Local edits that were not pushed are not part of the submission.

Each submission carries an operation id kept in .hearthroom/state.json until
the site accepts it, so running submit again after a lost reply does not submit
twice. --fandom names the work the card is based on, if any.

Updates are silent by default. --major-update requests a notification to people
who follow the author or saved the card, after the reviewed version goes live.
It requires --update-note: a plain-text change log of at least 10 non-whitespace
Unicode code points and at most 200 code points in total. Line endings become
LF and surrounding whitespace is removed; internal spaces and line breaks count
toward the maximum. Notifications for the same card are limited to once every
7 days; the site enforces this limit. Saving with card push does not notify.

Submitting publishes the card once reviewers approve it and may use your weekly
listing quota. Run it only when the author asks.

## Usage

```
hearthroom card submit [dir] [flags]
```

## Options

```
      --fandom string        the work the card is based on, if any
      --major-update         notify followers and people who saved the card after the reviewed update goes live (requires --update-note)
      --update-note string   plain-text change log for --major-update (10 non-whitespace Unicode code points minimum, 200 total maximum)
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
