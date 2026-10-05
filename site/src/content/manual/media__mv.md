---
slug: "media/mv"
command: "hearthroom media mv"
short: "Rename or move a folder or file in your media library"
parent: "media"
---

# hearthroom media mv

Renames a folder (and everything below it) or a file. A file's URL is its
path, so moving it changes the URL and the old URL stops working.

Without --yes this only shows what would move and which of your cards still
mention the old URLs; fix those cards (or accept that their images break) and
run again with --yes. A published card version can no longer be edited.

<from> is a folder ("my-card", "my-card/art") or a file path
("my-card/art/bg.webp") as "media ls" shows it; <to> is the new full path.

## Usage

```
hearthroom media mv <from> <to> [flags]
```

## Options

```
      --yes   move now, even if cards still mention the old URLs
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
