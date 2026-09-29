---
slug: "media/rm"
command: "hearthroom media rm"
short: "Delete files from your media library by id (see `media ls`)"
parent: "media"
---

# hearthroom media rm

Deletes library items by the id shown in "media ls". An item that one of your
cards still uses as portrait or background cannot be deleted; change the card's
image first.

## Usage

```
hearthroom media rm <id>...
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom media`](/manual/media/) — Upload and list files in your media library
