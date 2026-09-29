---
slug: "media/ls"
command: "hearthroom media ls"
short: "List files in your media library"
parent: "media"
---

# hearthroom media ls

List files in your media library

## Usage

```
hearthroom media ls [flags]
```

## Options

```
      --kind string    image, video, audio, font or all (default "all")
      --limit int      entries per page (default 50)
      --page int       page number (default 1)
      --q string       file name substring
      --scope string   role, folder or unfiled
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://hearthroom.club)
```

## See also

- [`hearthroom media`](/manual/media/) — Upload and list files in your media library
