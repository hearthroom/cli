---
slug: "workshop"
command: "hearthroom workshop"
short: "Create and edit workshop extensions, or publish your own author DLC"
parent: "hearthroom"
---

# hearthroom workshop

Manage extensions through the connected provider's Workshop API.
An original author can add openings, response formatting rules and Lorebook entries
as DLC. Other creators can add Lorebook entries only. The provider derives the kind
from ownership and checks the original author's opt-in (closed by default).

Writes require role.write; reads require role.read. These commands read your own
extension drafts, never the original agent configuration's private instructions,
rules or Lorebook. Publication has no content review or revenue share.

Create, update, publish and unpublish require --operation-id. Use a new token
for each intent and reuse it only to retry the exact same request.
Updates and publication actions require --expected-revision to
protect concurrent edits. Publication returns a provider receipt; community-site
projection may become available later. Every command accepts --json.

## Commands

- [`hearthroom workshop create`](/manual/workshop/create/) — Create an empty extension draft for an agent configuration
- [`hearthroom workshop get`](/manual/workshop/get/) — Read your extension draft as JSON, including its revision
- [`hearthroom workshop list`](/manual/workshop/list/) — List your own Workshop drafts and publications
- [`hearthroom workshop policy`](/manual/workshop/policy/) — Read an agent configuration's Workshop opt-in policy
- [`hearthroom workshop policy-set`](/manual/workshop/policy-set/) — Change Workshop opt-in for your original agent configuration
- [`hearthroom workshop publish`](/manual/workshop/publish/) — Publish your own author DLC
- [`hearthroom workshop unpublish`](/manual/workshop/unpublish/) — Unpublish your own author DLC and retain its draft
- [`hearthroom workshop update`](/manual/workshop/update/) — Replace your draft's editable content with a JSON file

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
