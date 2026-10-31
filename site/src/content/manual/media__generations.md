---
slug: "media/generations"
command: "hearthroom media generations"
short: "List your recent image generations and which images are saved"
parent: "media"
---

# hearthroom media generations

Lists the image generations of the last 7 days, newest first, from this CLI,
hearthroom.club and any other signed-in client. Each image shows its number
(for "media save --pick"), its result URL and whether it is already in your
media library. Results that are not saved are deleted after 7 days.

## Usage

```
hearthroom media generations [flags]
```

## Options

```
      --limit int   how many generations to list (1-100) (default 20)
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom media`](/manual/media/) — Upload, list and generate files in your media library
