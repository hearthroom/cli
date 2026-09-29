---
slug: "completion/zsh"
command: "hearthroom completion zsh"
short: "Generate the autocompletion script for zsh"
parent: "completion"
---

# hearthroom completion zsh

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(hearthroom completion zsh)

To load completions for every new session, execute once:

#### Linux:

	hearthroom completion zsh > "${fpath[1]}/_hearthroom"

#### macOS:

	hearthroom completion zsh > $(brew --prefix)/share/zsh/site-functions/_hearthroom

You will need to start a new shell for this setup to take effect.

## Usage

```
hearthroom completion zsh [flags]
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
