---
slug: "workshop/publish"
command: "hearthroom workshop publish"
short: "Publish your draft directly, without content review"
parent: "workshop"
---

# hearthroom workshop publish

Publish your draft directly, without content review

## Usage

```
hearthroom workshop publish <workshopId> [flags]
```

## Options

```
      --expected-revision int   require this draft revision (required)
      --operation-id string     stable intent token for retrying this action (required)
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
