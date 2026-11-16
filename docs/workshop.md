# Workshop extensions

`hearthroom workshop` manages extensions through the connected Provider's
existing `/open/v1/workshop/*` API. Sign in with `hearthroom auth login` first.
Reads require `role.read`; writes require `role.write`.

The Provider derives the extension kind from original ownership:

- **Author DLC** (`kind: "dlc"`): original authors can supply openings,
  response formatting rules and Lorebook entries.
- **Player extensions** (`kind: "lorebook"`): other creators can supply
  Lorebook entries only. They cannot read or replace the original agent
  configuration's private instructions, rules or Lorebook.

An extension can have at most **1,500 always-included characters**. Triggered
entries use the same recall pool as other Lorebook entries and add content;
they do not replace original entries. The original author's opt-in is closed
by default. Publication requires no content review and earns no revenue share.
The Provider still enforces identity, structure, capacity and author policy.

## Create, edit and publish

Use the Provider's role UUID for `<roleId>`, and the returned Workshop UUID for
`<workshopId>`. `workshop list` lists your own drafts and publications, including
their server-derived `kind` and state.

```sh
hearthroom workshop list --json
hearthroom workshop create <roleId> --operation-id <create-intent> --json
hearthroom workshop get <workshopId> --json > extension.json
# Edit extension.json. Keep its workshopId and revision unchanged.
hearthroom workshop update <workshopId> --file extension.json \
  --expected-revision <draft-revision> --operation-id <save-intent> --json
hearthroom workshop publish <workshopId> \
  --expected-revision <saved-revision> --operation-id <publish-intent> --json
```

Choose a new operation token (1–100 UTF-8 bytes) for each intent and retain it
with the request. To retry after a lost response, reuse the **same token,
revision and content**. Do not reuse a token for a changed request. A stale
revision is an error: read the latest draft, reconcile your edits and submit
a new intent. The CLI does not silently retry writes or overwrite conflicts.

`update` replaces the entire editable draft, rather than patching individual
fields. Omitted fields are cleared. It accepts the JSON from `get`, forwards
only editable extension fields, and checks that the saved identity and revision
match the command. Read-only `kind`, `card`, `published` and
`changedSincePublish` metadata is ignored. Unknown fields are rejected.

Editable fields are `name`, `summary`, `readme`, `coverUrl`, `rating`,
`ratingAnswers`, `worldbook`, `rules`, `opening`, and `updateNote`. A minimal
player extension payload can be:

```json
{
  "name": "The harbor district",
  "summary": "Locations for a fictional coastal city.",
  "rating": "G",
  "worldbook": [
    {
      "id": "harbor",
      "name": "Harbor",
      "content": "A bell marks the tide at the harbor.",
      "keywords": ["harbor"],
      "enabled": true,
      "mode": "triggered"
    }
  ]
}
```

Player extensions omit `rules` and `opening`; the empty structures returned
by `get` are also accepted. Author DLC can use the Provider's `rules` and
`opening` DTOs. Local media paths are not uploaded or rewritten by these
commands; use already-hosted URLs owned by the signed-in creator.

Publication returns the Provider receipt. Community-site projection is
asynchronous; this receipt alone does not prove the item is visible there.

## Unpublish

```sh
hearthroom workshop get <workshopId> --json
hearthroom workshop unpublish <workshopId> \
  --expected-revision <current-draft-revision> --operation-id <unpublish-intent> --json
```

This withdraws the publication while retaining the draft. It does not delete
the original agent configuration or rewrite past conversations.

## Original-author opt-in

```sh
hearthroom workshop policy <roleId> --json
hearthroom workshop policy-set <roleId> --allow-new=true \
  --expected-revision <policy-revision> --json
```

Only the original author can change policy. Use `--allow-new=false` to close
it. A missing policy has revision zero and is closed; zero is valid for the
first policy write. Policy revisions and draft revisions are separate.

All commands accept `--json`. Human output uses the same readable JSON DTOs,
so its identity, revision and validation results remain visible.
