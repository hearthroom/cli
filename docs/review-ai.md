# Card review tools

`hearthroom review` lets an automated reviewer read card review submissions
and record suggestions for them. It calls the community site's
`/v1/review-ai/*` API.

Every command except `report` authenticates with a fixed reviewer key
taken from the `HEARTHROOM_REVIEW_KEY` environment variable, sent as the
`X-Review-Agent-Key` header; no sign-in is needed and nothing expires. `report`
is for signed-in managers (`hearthroom auth login`) and never sends the key.
The server decides who may use each command; anyone else gets an error.

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
hearthroom review originality <submissionId>
hearthroom review author <submissionId>
hearthroom review author-cards <submissionId>
hearthroom review card <cardNumber>
hearthroom review author-note <submissionId> [--set TEXT | --file PATH|-]
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
  "author": { "tier": "…", "batchSuspected": false },   // null once the card is decided
  "card": { },
  "stats": { },
  "allowed": { "verdicts": [], "reasons": [], "flags": [], "handoffCauses": [] }
}
```

Use the codes in `allowed`; the CLI does not keep its own list.

`show` also reads decided submissions older than 7 days (`phase: "closed"`), for
example the latest submission of a card listed by `author-cards`. Suggestions and
tags can only be written while the submission is pending or within 7 days of
submission (`410 review_ai_window_closed` otherwise).

### Evidence commands

These print the same evidence a human reviewer sees on the review page. They
never include account ids, device ids or reviewer names, and none of them reveals
the human decision on the submission being judged.

| Command | Returns | When to use |
|---|---|---|
| `originality <submissionId>` | Overall setting similarity; for each similar card by another author: card number, name, similarity, whether it entered review earlier, status (`approved`, `pending`, `removed`) and the overlapping passages quoted from this submission (4,000 characters in all, `overlapChars` is the full length) | `show` reports high originality and you need to see what overlaps and with which card |
| `author <submissionId>` | Account age; earlier approved and rejected submissions with a count per rejection reason; new-card and live-card-update submissions in the last 7 and 30 days; new-card submissions in the last 24 hours; accounts on the registering device; while pending, the author's tier, suspected batch submission and reviewers' author marks with their notes; your author note | Judging mass production, quota farming or a pattern of rejections |
| `author-cards <submissionId>` | The author's other cards (not this one): card number, name, status, live or not, latest submission (id, kind, status, times, length) and `settingSimilarity` between this setting and that card | Checking whether the author reuses one setting template across cards; finding submissions to open with `show` |
| `card <cardNumber>` | The card's live version in the same `card`/`stats` shape as `show`; `404 card_not_live` when it is not approved or is hidden | Comparing with another card from `author-cards` or `originality` |
| `author-note <submissionId>` | Your note on the submission's author (`{ note, updatedAt }`); `--set TEXT` or `--file PATH` (`-` for stdin, plain text) replaces it, up to 2,000 characters, and an empty note deletes it | Keeping what you learned about an author for later reviews; one note per author, never shown to authors |

Counts in `author` run up to `asOf`, which is now for a pending submission and the
submission time for a decided one. `settingSimilarity` is `null` when the other
card's text cannot be compared (rejected versions are not kept).

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
