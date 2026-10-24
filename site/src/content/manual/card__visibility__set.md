---
slug: "card/visibility/set"
command: "hearthroom card visibility set"
short: "Change visibility without submitting for review"
parent: "card/visibility"
---

# hearthroom card visibility set

Changes access for the linked owned card. private lets the author and previous
players continue conversations or start new ones, with normal charges. People
who have never played the card cannot open it; it stays out of public listings.
Previous players retain play access to versions they have played and approved
versions of the same work. A source version played while unlisted also remains
playable.
Other unplayed drafts or versions awaiting review remain private. Play access
does not grant access to private instructions or permission to export the
configuration. unlisted shares the latest saved content with anyone who has
the link, without listing it. public restores an unchanged approved version;
review_required means the author must explicitly request card submit.
This command never submits for review.

An unresolved operation is kept in .hearthroom/state.json. Use visibility get
to check it, or repeat the same set to resume with the original operation id.

## Usage

```
hearthroom card visibility set <private|unlisted|public> [dir]
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card visibility`](/manual/card/visibility/) — Read or change who can access the linked card
