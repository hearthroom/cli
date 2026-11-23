# Card review tools

`hearthroom review` lets an automated reviewer read card review submissions
and record suggestions for them. It calls the community site's
`/v1/review-ai/*` API with the account signed in through
`hearthroom auth login`. The server decides which accounts may use each
command; other accounts get an error.

A suggestion is advisory only. It is stored next to the submission and does not
approve, reject or change the card.

Every command prints the server's JSON response on stdout. When the server
answers with an error (`{"error": "<code>", "field": "<name>"}`), the command
exits non-zero and prints the method, path, status, code and field on stderr.
With the global `--json` flag, that error is printed as a JSON object on stdout
instead, as for every other command.

## Commands

```sh
hearthroom review queue [--scope pending|recent] [--unshadowed] [--criteria v1] [--count] [--limit 50]
hearthroom review show <submissionId>
hearthroom review suggest <submissionId> --criteria v1 --model <id> --verdict pass|reject|handoff \
  [--reason code]... [--flag code]... [--confidence 0.8] [--note text] [--handoff-cause cause] \
  [--input-tokens n] [--output-tokens n] [--cost-usd x] [--latency-ms n]
hearthroom review suggest <submissionId> --file body.json [--criteria v1] [--model <id>]
hearthroom review tag <submissionId> --criteria v1 --model <id> --flag code...
hearthroom review report [--from ms] [--to ms] [--criteria v1]
```

### queue

`GET /v1/review-ai/queue`. `--scope pending` (default) lists submissions waiting
for a decision; `recent` lists those submitted within 7 days, decided or not.
`--unshadowed` keeps only submissions you have not given a verdict for, for the
`--criteria` version when given. `--limit` is 1–200.

```json
{ "items": [{ "id": "…", "cardId": "…", "kind": "first", "reopen": false, "submittedAt": 1700000000000, "rating": "…" }] }
```

With `--count`: `{ "count": 3 }`.

### show

`GET /v1/review-ai/submissions/<id>` returns the submission, a short author
summary (or `null`), the card, its stats, and the codes the server accepts:

```json
{
  "submission": { "id": "…", "cardId": "…", "kind": "…", "reopen": false, "submittedAt": 0, "rating": "…", "attempt": 1, "previous": null, "phase": "…" },
  "author": { "tier": "…", "batchSuspected": false },
  "card": { },
  "stats": { },
  "allowed": { "verdicts": [], "reasons": [], "flags": [], "handoffCauses": [] }
}
```

Use the codes in `allowed`; the CLI does not keep its own list.

### suggest

`PUT /v1/review-ai/submissions/<id>/suggestion` with:

```json
{
  "criteriaVersion": "v1",
  "model": "<model id>",
  "verdict": "pass | reject | handoff",
  "reasons": ["<code>"],
  "flags": ["<code>"],
  "confidence": 0.8,
  "note": "up to 400 characters",
  "handoffCause": "unsure | refusal | invalid_output | timeout | error | too_large",
  "inputTokens": 0,
  "outputTokens": 0,
  "costUsd": 0,
  "latencyMs": 0
}
```

`handoffCause` and the usage fields are optional. The response is
`{ "id", "verdict", "phase", "criteriaVersion" }`. Sending again for the same
submission and criteria version replaces your earlier suggestion.

Build the body from flags, or pass a JSON object with `--file <path>` (`-`
reads stdin). Flags given on the command line replace the matching fields of
the file, so an agent can pipe the model's JSON and add its own fields:

```sh
model-output | hearthroom review suggest <submissionId> --file - --criteria v1 --model <id>
```

The CLI only checks that the verdict is `pass`, `reject` or `handoff` and that
the criteria version looks like `v1`; the server validates everything else.

### tag

`PUT /v1/review-ai/submissions/<id>/tags` with
`{ "criteriaVersion", "model", "flags": [] }`. It sets flags and keeps any
verdict already recorded. The response has the same shape as `suggest`.

### report

`GET /v1/review-ai/report` summarizes suggestions by criteria version between
`--from` and `--to` (Unix milliseconds). Only review managers may read it. The
response is `{ "from", "to", "versions": [] }`.
