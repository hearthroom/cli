---
slug: "review"
command: "hearthroom review"
short: "Staff tools for card review (the server decides who may use them)"
parent: "hearthroom"
---

# hearthroom review

Staff tools for card review submissions: list the queue, read a submission,
and record an advisory suggestion or flags for one. Suggestions do not decide
a review. The server decides which accounts may use each command.

Every command prints the server's JSON response.

## Commands

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
