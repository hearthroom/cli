---
slug: "workshop/update"
command: "hearthroom workshop update"
short: "Replace your draft's editable content with a JSON file"
parent: "workshop"
---

# hearthroom workshop update

Replace all editable content in an extension draft (not a partial patch).
Start with "workshop get <workshopId> --json", edit that JSON, and pass the saved
revision with --expected-revision. Omitted fields are cleared by the provider.
Player extensions must omit rules and opening (empty structures from get are safe).
DLC drafts may also carry contexts and their own card tools; contexts can name only
those tools. Player extensions omit both.
Use --file - to read from stdin. Local media paths are not uploaded or rewritten.

## Usage

```
hearthroom workshop update <workshopId> --file <draft.json> [flags]
```

## Options

```
      --context-root string     DLC folder containing contexts/ and private preset files; overrides contexts in --file
      --expected-revision int   require this draft revision before writing (required)
      --file string             complete editable draft JSON, or '-' for stdin (required)
      --operation-id string     stable intent token for retrying this update (required)
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom workshop`](/manual/workshop/) — Create and edit workshop extensions, or publish your own author DLC

## Author DLC Lorebook packages

Use `workshop get --json` to edit your own author DLC. Each DLC accepts up to
10,000 packages. Each `lorePacks` item has
a stable UUID `id`, a unique `name` (up to 12 characters), an optional
`description` (up to 24), and a `defaultEnabled` boolean. Players can see package
names and descriptions. Set an entry's `packId` to put it in a package. An entry
without a `packId` follows the DLC's switch. Move entries together by editing
their `packId` values in one draft update.

Keep package UUIDs when changing names or descriptions. When removing a package,
remove its `packId` from the affected entries in the same update. Pass
`lorePacks: []` to clear all packages. Older update files that omit `lorePacks`
preserve package assignments on entries with matching IDs. Publish the saved
draft to make changes available in play.

Set `scriptCanSwitchPacks: true` to let the card's own script change the package
selection for the current conversation. The setting defaults to off. Most Lorebook
entries should use trigger words to keep the always-included context small.
