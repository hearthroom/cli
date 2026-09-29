---
slug: "completion"
command: "hearthroom completion"
short: "Shell completion: install, uninstall, or print the script"
parent: "hearthroom"
---

# hearthroom completion

Generate the autocompletion script for hearthroom for the specified shell.
See each sub-command's help for details on how to use the generated script.

## Commands

- [`hearthroom completion bash`](/manual/completion/bash/) — Generate the autocompletion script for bash
- [`hearthroom completion fish`](/manual/completion/fish/) — Generate the autocompletion script for fish
- [`hearthroom completion install`](/manual/completion/install/) — Set up tab completion for your shell
- [`hearthroom completion powershell`](/manual/completion/powershell/) — Generate the autocompletion script for powershell
- [`hearthroom completion uninstall`](/manual/completion/uninstall/) — Remove the completion files and rc block written by install
- [`hearthroom completion zsh`](/manual/completion/zsh/) — Generate the autocompletion script for zsh

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
