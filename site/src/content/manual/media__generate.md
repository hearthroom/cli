---
slug: "media/generate"
command: "hearthroom media generate"
short: "Generate images with a model and optionally save them to your media library"
parent: "media"
---

# hearthroom media generate

Generates images from a prompt (see "media models" for models, prices and
sizes). It spends purchased credits per image, so it requires --allow-spend;
granted or gift credits cannot pay for it. Refused prompts and failed
generations are not charged.

Results are kept for 7 days. To keep them, save them to your media library
with --dir <path> (created if missing, e.g. the card's folder name) or --folder
<name or id> (an existing folder), and/or download them with --out <file>.
To look first and keep only some, leave both out, then use "media generations"
and "media save --pick".
A saved image prints its library URL, which a card can use directly, e.g. as
media.portrait in card.json; a file downloaded into the card's assets/ is
referenced by its relative path instead.

To keep a character looking the same across images, pass its portrait with
--image (a library URL, or its path in the library as "media ls" shows it;
repeatable, up to the model's REFS in "media models").
Reference images cost nothing extra.

Each run sends an Idempotency-Key. If the command stops waiting, run it again
with the printed --idempotency-key to pick up the same generation without
being charged twice.

## Usage

```
hearthroom media generate [flags]
```

## Options

```
      --allow-spend              confirm that this generation may spend purchased credits
  -n, --count int                number of images (up to the model's MAX) (default 1)
      --dir string               save to this media library path, e.g. the card's folder name
      --folder string            save to an existing media library folder, by name or id
      --idempotency-key string   reuse a key to resume the same generation without paying twice (default: a new key)
      --image stringArray        reference image from your media library, as a URL or library path (repeatable); keeps a character consistent
      --model string             model id from "hearthroom media models"
      --out string               also download to this file; with -n above 1, -1, -2… are added before the extension
      --prompt string            what to draw (at most 4000 characters)
      --prompt-file string       read the prompt from a file, or - for stdin
      --size string              one of the model's sizes, e.g. 1024x1536 (default: the model's)
      --timeout duration         how long to wait for a slow generation (default 10m0s)
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
