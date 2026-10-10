---
slug: "card/check"
command: "hearthroom card check"
short: "Local checks of the folder before a push: rules, markers, sandbox API use, protocol health"
parent: "card"
---

# hearthroom card check

Reads the folder and reports what the provider's validate and render cannot see,
without signing in or sending anything:

  - rules.json: invalid or empty-matching patterns, flags outside gimsuy, blank find,
    duplicate ids, a replacement over 128 KiB (UTF-8 bytes) or a set over 32 MiB,
    sdk capabilities and event names the sandbox page does not have, sdk.off/once,
    sdk.vars, module syntax, an await before sdk.message.send, invalid save keys,
    attributes the sanitizer strips (data-*, aria-*, role; on* inside <svg>), tags with
    Chinese characters, hc-* components, {{random:a|b}}, asset paths built at runtime
    or pointing at missing files, and the sandbox API on a classic-page card;
  - render rules are not generation rules: a marker a rule consumes must be something
    the definition, the output contract, a constant Lorebook entry or an opening tells
    the model to write, or the panel appears once and never updates;
  - README.md (never sent) declarations: uiRole: assist | core and, for a core card,
    statusOverheadThreshold;
  - rating.json (never sent): whether the content rating that card submit needs is
    there and has the shape this build knows (card rate writes it).

--replay reads replies ("hearthroom play --history" text or --json output,
preview/replies.md with "## " headings, or paragraphs) and reports protocol health:
how often the [status] block is written, missing closers, full-width punctuation,
the status overhead ratio against the declared threshold (15% by default), and the
keys the model forgets. Every fact comes from the sandbox contract embedded in this
build. Exit code 1 when there are errors; warnings do not fail.

## Usage

```
hearthroom card check [dir] [--replay file...] [flags]
```

## Options

```
      --replay stringArray   a transcript of replies to measure the status protocol against (repeatable)
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
