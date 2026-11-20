---
slug: "workshop/unpublish"
command: "hearthroom workshop unpublish"
short: "Unpublish your own author DLC and retain its draft"
parent: "workshop"
---

# hearthroom workshop unpublish

Unpublish your own author DLC and retain its draft

## Usage

```
hearthroom workshop unpublish <workshopId> [flags]
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

- [`hearthroom workshop`](/manual/workshop/) — Create and edit workshop extensions, or publish your own author DLC
