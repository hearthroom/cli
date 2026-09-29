---
slug: "search"
command: "hearthroom search"
short: "Browse or search the community board (no sign-in needed)"
parent: "hearthroom"
---

# hearthroom search

Browse or search the community board (no sign-in needed)

## Usage

```
hearthroom search [text] [flags]
```

## Options

```
      --author string     author handle (ignores --zone)
      --lang string       display language for card text
      --limit int         entries per page (default 20)
      --offset int        pagination offset
      --sort string       new or top
      --tag stringArray   require a tag (repeatable)
      --zone string       content language zone: zh, en, ja, ko or all (site default: zh)
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://hearthroom.club)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
