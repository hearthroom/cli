---
slug: "completion/install"
command: "hearthroom completion install"
short: "Set up tab completion for your shell"
parent: "completion"
---

# hearthroom completion install

Detects your shell (or takes --shell), writes the completion script where the
shell loads it from, and adds a small marked block to your shell's rc file so
completion is active in new terminals. Running it again is safe; it replaces
the block it wrote before.

  bash        ~/.local/share/bash-completion/completions/hearthroom, plus a block in ~/.bashrc
  zsh         a block in ~/.zshrc that loads the script from the binary
  fish        ~/.config/fish/completions/hearthroom.fish (no rc change needed)
  powershell  prints the line to add to your $PROFILE

## Usage

```
hearthroom completion install [flags]
```

## Options

```
      --no-rc          write the completion file but do not edit any rc file
      --print          show what would be written without changing anything
      --rc string      rc file to edit instead of the default (~/.bashrc or ~/.zshrc)
      --shell string   bash, zsh, fish or powershell (default: detected from $SHELL)
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
