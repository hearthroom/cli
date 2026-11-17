---
slug: "workshop/policy-set"
command: "hearthroom workshop policy-set"
short: "Change Workshop opt-in for your original agent configuration"
parent: "workshop"
---

# hearthroom workshop policy-set

Change Workshop opt-in for your original agent configuration

## Usage

```
hearthroom workshop policy-set <roleId> [flags]
```

## Options

```
      --allow-new               allow new extensions; original-author access required
      --expected-revision int   require this policy revision (required; zero is valid)
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
