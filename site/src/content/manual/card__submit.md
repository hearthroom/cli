---
slug: "card/submit"
command: "hearthroom card submit"
short: "Submit the linked card for review on the community site, with its content rating"
parent: "card"
---

# hearthroom card submit

Sends the card the folder is linked to, as last pushed, for review on the
community site, together with the answers in rating.json (see card rate). Only
a card you own can be submitted: a trial card cannot, so run
"hearthroom card push --create" first to make a private card from the folder.
Local edits that were not pushed are not part of the submission.

Each submission carries an operation id kept in .hearthroom/state.json until
the site accepts it, so running submit again after a lost reply does not submit
twice. --fandom names the work the card is based on, if any.

Submitting publishes the card once reviewers approve it and may use your weekly
listing quota. Run it only when the author asks.

## Usage

```
hearthroom card submit [dir] [flags]
```

## Options

```
      --fandom string   the work the card is based on, if any
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
