---
slug: "review/author"
command: "hearthroom review author"
short: "Show the submitting author's history, pace, reviewer marks and your note on them"
parent: "review"
---

# hearthroom review author

Shows evidence about the author of a submission, as numbers: account age in
days; earlier approved and rejected submissions with a count per rejection
reason; submissions of new cards and of updates to live cards in the last 7 and
30 days; new-card submissions in the last 24 hours; and how many accounts use
the device that registered this card. While the submission is pending it also
gives the author's tier, whether batch submission is suspected, and the marks
reviewers set on the author with their reason and note. Your own note on the
author is included (see author-note).

Counts run up to asOf: now for a pending submission, the submission time for a
decided one. Use it to judge mass production or a pattern of rejections; put
what you learn in your suggestion's reviewer note, not in the note to the author.

## Usage

```
hearthroom review author <submissionId>
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
