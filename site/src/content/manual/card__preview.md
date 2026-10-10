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

--check gives an agent without a browser the same evidence: it drives headless Chrome
through the harness, streams the sample replies, and for a fixed set of states (phone
and desktop, dark and light, rules on and off, the first and the last sample) saves a
screenshot under preview/shots/, reads the facts a picture does not show (when the
status panel was drawn after the reply finished, whether the phone width scrolls
sideways, choice buttons, text under 12px, the card scripts' console errors) and writes
preview/shots/findings.json plus a contact sheet, preview/shots/contact.png, with every
shot and its numbers. Errors (a block written but never drawn as a panel, sideways
overflow at phone width, a script error) exit 1; warnings and info exit 0. Chrome,
Chromium or Edge is found on PATH or in the usual install locations, or named with
HEARTHROOM_CHROME. --from-history streams real replies from a "hearthroom play
--history" file instead of preview/replies.md; --sizes overrides the viewports.
Nothing under preview/ is ever pushed: card push sends the card files and the assets the
card references, so the shots stay on this machine.

The shell is fetched once from the card's sandbox origin on the configured site
(c<roleId>.<site host>/sandbox/) and cached by content hash; --shell <dir> uses a
local build of the shell instead. Nothing is sent anywhere else.

## Usage

```
hearthroom card preview [dir] [flags]
```

## Options

```
      --check                      drive headless Chrome: screenshots, facts and findings under preview/shots/, then exit
      --from-history stringArray   with --check: stream the replies in this play --history file instead of preview/replies.md (repeatable)
      --open                       open the preview in the default browser
      --port int                   local port (0 picks a free one) (default 4173)
      --shell string               serve a local sandbox shell build (dist-sandbox) instead of fetching the site's
      --sizes string               with --check: viewports as WxH,WxH (default 390x844,1280x800)
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, check, push, validate, render, preview, pull, import, list, rate, submit
