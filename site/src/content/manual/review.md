---
slug: "review"
command: "hearthroom review"
short: "Staff tools for card review (the server decides who may use them)"
parent: "hearthroom"
---

# hearthroom review

Staff tools for card review submissions: list the queue, read a submission
and the evidence around it, and record an advisory suggestion or flags for one.
Suggestions do not decide a review. The server decides which accounts may use
each command.

Evidence: originality (overlap with other authors' cards), author (the author's
history and pace), author-cards (the author's other cards), card (a card's live
version) and author-note (your own notes on the author).

Every command prints the server's JSON response.

## Commands

- [`hearthroom review author`](/manual/review/author/) — Show the submitting author's history, pace, reviewer marks and your note on them
- [`hearthroom review author-cards`](/manual/review/author-cards/) — List the author's other cards and how much of this setting each one shares
- [`hearthroom review author-note`](/manual/review/author-note/) — Read or replace your private note on the submission's author
- [`hearthroom review card`](/manual/review/card/) — Read the version of a card that is live now
- [`hearthroom review originality`](/manual/review/originality/) — List other authors' cards whose setting overlaps this submission, with the overlapping text
- [`hearthroom review queue`](/manual/review/queue/) — List review submissions
- [`hearthroom review report`](/manual/review/report/) — Summarize suggestions by criteria version over a time range
- [`hearthroom review show`](/manual/review/show/) — Read one review submission with its card and the allowed suggestion codes
- [`hearthroom review suggest`](/manual/review/suggest/) — Record an advisory suggestion for a review submission
- [`hearthroom review tag`](/manual/review/tag/) — Set flag codes on a review submission without changing your verdict

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
