---
slug: "completion/fish"
command: "hearthroom completion fish"
short: "Generate the autocompletion script for fish"
parent: "completion"
---

# hearthroom completion fish

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	hearthroom completion fish | source

To load completions for every new session, execute once:

	hearthroom completion fish > ~/.config/fish/completions/hearthroom.fish

You will need to start a new shell for this setup to take effect.

## Usage

```
hearthroom completion fish [flags]
```

## Options

```
      --no-descriptions   disable completion descriptions
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom completion`](/manual/completion/) — Shell completion: install, uninstall, or print the script
