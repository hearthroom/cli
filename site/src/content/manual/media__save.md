---
slug: "media/save"
command: "hearthroom media save"
short: "Save picked images of a generation to your media library, under names you choose"
parent: "media"
---

# hearthroom media save

Saves images of a generation (see "media generations") into your media
library with --dir <path> (created if missing, e.g. the card's folder name) or
--folder <name or id> (an existing folder).

--pick takes the image numbers shown by "media generations", starting at 1;
repeat it or separate numbers with commas. Without --pick every image is saved.
--name gives the file name for each picked image, in the same order; the
extension is added for you. A name already used in that folder is refused,
never overwritten. Saving an image again returns the file it was saved as.
Saving is free and uses your media library storage.

## Usage

```
hearthroom media save <generation-id> [flags]
```

## Options

```
      --dir string         save to this media library path, e.g. the card's folder name
      --folder string      save to an existing media library folder, by name or id
      --name stringArray   file name for each picked image, in --pick order (default: model and generation id)
      --pick ints          image number from "media generations", starting at 1 (repeatable or comma-separated; default: all)
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
