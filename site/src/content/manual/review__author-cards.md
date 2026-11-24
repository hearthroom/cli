---
slug: "review/author-cards"
command: "hearthroom review author-cards"
short: "List the author's other cards and how much of this setting each one shares"
parent: "review"
---

# hearthroom review author-cards

Lists the author's other cards, newest first: card number, name, status,
whether it is live, and its latest submission (id, kind, status, times and
length). This submission's own card is not listed.

settingSimilarity (0 to 1) is how much of this submission's setting also appears
in that card. Several cards near 1 mean one setting template reused under new
names. Only the 30 most recently registered cards are compared. null means that
card was not compared or its text is not kept (rejected versions are dropped).

Open a card's latest submission with show <latest.id>, or its live version with
card <cardId>.

## Usage

```
hearthroom review author-cards <submissionId>
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
