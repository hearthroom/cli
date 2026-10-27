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

The default is a silent update, with no update notification.

Leading and trailing whitespace is trimmed. A changelog needs at least 10 non-whitespace Unicode code points and no more than 200 code points in total; internal whitespace and line breaks count toward the total limit.

Each card can send one update notification every 3 days; silent updates remain available between notifications.

Notifications follow each recipient’s notification settings.

Submitting publishes the card once reviewers approve it and may use your weekly
listing quota. Run it only when the author asks.

## Usage

```
hearthroom card submit [dir] [flags]
```

## Options

```
      --fandom string        the work the card is based on, if any
      --notify               Send an update notification to people who saved this card or follow its creator after the new version is approved and published; requires --update-note.
      --update-note string   Public changelog shown in the notification: at least 10 characters excluding whitespace, with a total limit of 200; requires --notify.
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
