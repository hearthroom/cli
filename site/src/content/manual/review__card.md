---
slug: "review/card"
command: "hearthroom review card"
short: "Read the version of a card that is live now"
parent: "review"
---

# hearthroom review card

Reads the version of a card that players see now, in the same card and stats
shape as show. Fails with card_not_live when the card is not approved or is
hidden from the public.

Use it for other cards, from author-cards or originality, to compare them with
the submission you are judging. To read a submission itself, use show.

## Usage

```
hearthroom review card <cardNumber>
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom review`](/manual/review/) — Staff tools for card review (the server decides who may use them)
