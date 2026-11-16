---
slug: "card/review-status"
command: "hearthroom card review-status"
short: "Read your card's published version and latest review submission"
parent: "card"
---

# hearthroom card review-status

Reads the current public version and latest review submission for a card you own.
Use a linked owned card folder (current directory by default), or --role with
its provider role ID or community card number. This only reads status; it does
not push, submit, change visibility or wait for a review to finish.

The latest submission is separate from the public version: an approved listing
can still have a pending or rejected update. JSON includes the submission ID,
versionId, submittedAt (Unix milliseconds), kind (first or re), status,
rejectionReason, updateKind and silentUpdate. Missing history or unavailable
legacy version/update metadata is null. Superseded submissions keep their
actual status. Only the author's visible rejection reason is returned.

## Usage

```
hearthroom card review-status [dir] [flags]
```

## Options

```
      --role string   owned provider role ID or community card number (instead of a folder)
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
