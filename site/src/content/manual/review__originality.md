---
slug: "review/originality"
command: "hearthroom review originality"
short: "List other authors' cards whose setting overlaps this submission, with the overlapping text"
parent: "review"
---

# hearthroom review originality

Lists the cards by other authors whose character setting overlaps this
submission's setting: overall similarity, then for each similar card its card
number, name, similarity (0 to 1), whether it entered review earlier, its status
(approved, pending or removed) and the overlapping passages quoted from this
submission (at most 4,000 characters in all; overlapChars is the full length).

Use it when show reports a high originality similarity, to read what was copied
and from whom. The passages come from this submission, so they stay readable when
the other card is pending or removed. Open a listed card's live version with card.
The author's own cards are not listed; see author-cards for those.

## Usage

```
hearthroom review originality <submissionId>
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
