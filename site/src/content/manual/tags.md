---
slug: "tags"
command: "hearthroom tags"
short: "List community tags or platform preset category tags"
parent: "hearthroom"
---

# hearthroom tags

Lists tags used on public community cards, or with --preset lists the platform's
fixed preset category and play-style catalog.

Public card publication (card submit and card visibility set public) requires at
least one preset tag in card.json tags; custom free-form tags remain allowed
alongside preset tags and do not satisfy the requirement on their own. Write a
preset tag's display name (for example from --preset --lang <card.language>),
not its URL key, into card.json tags.

## Usage

```
hearthroom tags [flags]
```

## Options

```
      --lang string   display language for --preset tag names: zh-Hant, zh-Hans, en, ja, or ko
      --limit int     entries (default 40)
      --preset        list the platform preset category and play-style tags (at least one is required in card.json tags to publish publicly)
      --q string      tag search text
      --zone string   content language zone
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
