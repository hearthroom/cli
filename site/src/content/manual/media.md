---
slug: "media"
command: "hearthroom media"
short: "Upload, list and generate files in your media library"
parent: "hearthroom"
---

# hearthroom media

Upload, list and generate files in your media library

## Commands

- [`hearthroom media generate`](/manual/media/generate/) — Generate images with a model and optionally save them to your media library
- [`hearthroom media generations`](/manual/media/generations/) — List your recent image generations and which images are saved
- [`hearthroom media ls`](/manual/media/ls/) — List files in your media library
- [`hearthroom media models`](/manual/media/models/) — List image models for media generate, with price and sizes
- [`hearthroom media mv`](/manual/media/mv/) — Rename or move a folder or file in your media library
- [`hearthroom media rm`](/manual/media/rm/) — Delete files from your media library by id (see `media ls`)
- [`hearthroom media save`](/manual/media/save/) — Save picked images of a generation to your media library, under names you choose
- [`hearthroom media upload`](/manual/media/upload/) — Upload files to your media library

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
