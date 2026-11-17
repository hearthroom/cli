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

- [`hearthroom workshop`](/manual/workshop/) — Create, edit and publish author DLC or player Lorebook extensions
