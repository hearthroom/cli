---
slug: "card/preview"
command: "hearthroom card preview"
short: "Open the folder in the real sandbox chat shell with a fake host, offline"
parent: "card"
---

# hearthroom card preview

Serves the card folder to the community site's own sandbox shell on a local port,
with this CLI playing the host: the opening, display rules and scripts run exactly as
on the play page (the real sdk, sanitizer, Markdown and event order), without a push
and without credits. The page streams sample replies from preview/replies.md (one per
"## " heading), switches conversation, re-runs scripts, offers phone, landscape,
unfolded, tablet and desktop sizes, both themes, and "rules off" (?rules=off) to read
the card as plain text. It is not the host's own render path, the model or a device.

The shell is fetched once from the card's sandbox origin on the configured site
(c<roleId>.<site host>/sandbox/) and cached by content hash; --shell <dir> uses a
local build of the shell instead. Nothing is sent anywhere else.

## Usage

```
hearthroom card preview [dir] [flags]
```

## Options

```
      --open           open the preview in the default browser
      --port int       local port (0 picks a free one) (default 4173)
      --shell string   serve a local sandbox shell build (dist-sandbox) instead of fetching the site's
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, check, push, validate, render, preview, pull, import, list
