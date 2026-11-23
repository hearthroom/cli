---
slug: "review/suggest"
command: "hearthroom review suggest"
short: "Record an advisory suggestion for a review submission"
parent: "review"
---

# hearthroom review suggest

Records your suggestion for a submission: a verdict (pass, reject or handoff),
reason and flag codes, confidence and a short note. It is advisory only and does
not decide the review. Sending again for the same submission and criteria
version replaces your earlier suggestion.

Use flags, or --file with a JSON body (- reads stdin). Flags given on the command
line replace the matching fields of the file. Allowed codes are listed by
review show.

## Usage

```
hearthroom review suggest <submissionId> [flags]
```

## Options

```
      --confidence float       confidence from 0 to 1
      --cost-usd float         model cost in US dollars
      --criteria string        criteria version, e.g. v1
      --file string            JSON body to send ('-' for stdin)
      --flag stringArray       flag code (repeatable)
      --handoff-cause string   why the submission is handed to a person: unsure, refusal, invalid_output, timeout, error or too_large
      --input-tokens int       model input tokens
      --latency-ms int         model latency in milliseconds
      --model string           model that produced the suggestion
      --note string            short note (up to 400 characters)
      --output-tokens int      model output tokens
      --reason stringArray     reason code (repeatable)
      --verdict string         pass, reject or handoff
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
