---
slug: "card/render"
command: "hearthroom card render"
short: "Show an opening after the card's display rules, as the player's renderer receives it"
parent: "card"
---

# hearthroom card render

Runs the pushed card's display rules over one opening on the provider, with the same
engine the play page uses, and prints what the renderer receives before Markdown
expansion, how each rule fared, and a static scan of the result, including author-API calls the card's chat page does not
provide (the sandbox sdk on a classic-page card, for example). It never spends credits.

Use --opening N to render the Nth alternate opening (0 is the main one). Write the
result to a file with --html to open it in a browser. Layout, contrast and HTML card
components are only visible on the play page; the preview link is printed for that.

## Usage

```
hearthroom card render [dir] [flags]
```

## Options

```
      --html string   also write the rendered text to this file
      --opening int   which opening to render: 0 is the main one, N the Nth alternate
      --push          push the folder first so the render reflects the current files
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom card`](/manual/card/) — Work with card folders: init, push, validate, render, pull, import, list
