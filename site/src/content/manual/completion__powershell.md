---
slug: "completion/powershell"
command: "hearthroom completion powershell"
short: "Generate the autocompletion script for powershell"
parent: "completion"
---

# hearthroom completion powershell

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	hearthroom completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.

## Usage

```
hearthroom completion powershell [flags]
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
      --site string         community site (default from config or https://hearthroom.club)
```

## See also

- [`hearthroom completion`](/manual/completion/) — Shell completion: install, uninstall, or print the script
