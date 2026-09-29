---
slug: "completion/bash"
command: "hearthroom completion bash"
short: "Generate the autocompletion script for bash"
parent: "completion"
---

# hearthroom completion bash

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(hearthroom completion bash)

To load completions for every new session, execute once:

#### Linux:

	hearthroom completion bash > /etc/bash_completion.d/hearthroom

#### macOS:

	hearthroom completion bash > $(brew --prefix)/etc/bash_completion.d/hearthroom

You will need to start a new shell for this setup to take effect.

## Usage

```
hearthroom completion bash
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
